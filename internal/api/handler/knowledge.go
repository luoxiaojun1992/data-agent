package handler

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	task "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/luoxiaojun1992/data-agent/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/service/knowledge"
)

// KnowledgeHandler provides HTTP handlers for knowledge base operations.
type KnowledgeHandler struct {
	svc       knowledge.KnowledgeService
	queueRepo repository.QueueRepository // optional: nil when queue is unavailable
}

// NewKnowledgeHandler creates a knowledge base handler.
func NewKnowledgeHandler(svc knowledge.KnowledgeService) *KnowledgeHandler {
	return &KnowledgeHandler{svc: svc}
}

// SetQueueRepo injects the queue repository for async KB indexing.
func (h *KnowledgeHandler) SetQueueRepo(qr repository.QueueRepository) {
	h.queueRepo = qr
}

// UploadDoc creates a new knowledge document with optional file upload to GridFS.
// Accepts either a multipart "file" (text docs) or a "file_base64" form field
// (images), whichever is present.
func (h *KnowledgeHandler) UploadDoc(c *gin.Context) {
	userID, _ := c.Get("user_id")
	title := c.PostForm("title")
	fileName := c.PostForm("file_name")
	fileType := c.PostForm("file_type")
	sizeBytes := int64(0)
	if s := c.PostForm("size_bytes"); s != "" {
		_, _ = fmt.Sscanf(s, "%d", &sizeBytes)
	}

	var gridFSFileID string

	// Path 1: multipart file upload (text documents).
	file, header, err := c.Request.FormFile("file")
	if err == nil {
		defer file.Close()
		// SPEC-068: redact PII from the text file before storing it to GridFS,
		// so the raw file (and downstream chunks) never contain PII.
		data, rErr := io.ReadAll(file)
		if rErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "read file failed: " + rErr.Error()})
			return
		}
		redacted, redactErr := h.svc.RedactText(c.Request.Context(), string(data))
		if redactErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "PII redaction failed: " + redactErr.Error()})
			return
		}
		gridFSFileID, err = h.svc.UploadFile(fileName+"_"+header.Filename, header.Header.Get("Content-Type"), bytes.NewReader([]byte(redacted)))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "GridFS upload failed: " + err.Error()})
			return
		}
		if sizeBytes == 0 {
			sizeBytes = int64(len(redacted))
		}
	} else if base64Data := c.PostForm("file_base64"); base64Data != "" {
		// Path 2: base64 image upload (data URI prefix optional).
		if idx := strings.Index(base64Data, ";base64,"); idx >= 0 {
			base64Data = base64Data[idx+len(";base64,"):]
		}
		decoded, derr := base64.StdEncoding.DecodeString(base64Data)
		if derr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid base64 image: " + derr.Error()})
			return
		}
		mimeType := c.PostForm("mime_type")
		if mimeType == "" {
			mimeType = "image/png"
		}
		gridFSFileID, derr = h.svc.UploadFile(fileName, mimeType, bytes.NewReader(decoded))
		if derr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "GridFS upload failed: " + derr.Error()})
			return
		}
		if sizeBytes == 0 {
			sizeBytes = int64(len(decoded))
		}
	}

	doc, err := h.svc.CreateDoc(userID.(string), title, fileName, fileType, sizeBytes, gridFSFileID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Enqueue async indexing via queue (no task/task_run records).
	if h.queueRepo != nil && gridFSFileID != "" {
		if err := h.queueRepo.EnqueueRaw(c.Request.Context(), "kb_index", task.KBIndexPayload{
			DocID:        doc.ID,
			GridFSFileID: gridFSFileID,
		}); err != nil {
			log.Printf("[kb] failed to enqueue index job for doc=%s: %v", doc.ID, err)
		}
	}

	c.JSON(http.StatusCreated, doc)
}

// GetDoc retrieves a knowledge document.
func (h *KnowledgeHandler) GetDoc(c *gin.Context) {
	docID := c.Param("id")
	doc, err := h.svc.GetDoc(docID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, doc)
}

// DeleteDoc removes a knowledge document and its chunks (cascade).
func (h *KnowledgeHandler) DeleteDoc(c *gin.Context) {
	docID := c.Param("id")
	if err := h.svc.DeleteDoc(docID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted", "id": docID})
}

// ListDocs lists documents visible to the current user.
// System admin: all docs. Regular user: own docs + public docs.
func (h *KnowledgeHandler) ListDocs(c *gin.Context) {
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")
	page, pageSize := parsePage(c)
	isSystemAdmin := role == "system_admin"
	docs, total, err := h.svc.ListDocsByVisibility(userID.(string), isSystemAdmin, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"docs": docs, "total": total, "page": page, "page_size": pageSize})
}

// Search performs hybrid search on the knowledge base with permission filtering.
func (h *KnowledgeHandler) Search(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' required"})
		return
	}
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")
	isSystemAdmin := role == "system_admin"

	results, err := h.svc.Search(userID.(string), query, 5, isSystemAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"query": query, "results": results})
}

// SetPublicFlag toggles the is_public flag on a knowledge document.
func (h *KnowledgeHandler) SetPublicFlag(c *gin.Context) {
	docID := c.Param("id")
	var req struct {
		IsPublic bool `json:"is_public"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.SetPublicFlag(c.Request.Context(), docID, req.IsPublic); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "updated", "is_public": req.IsPublic})
}

func parsePage(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}
