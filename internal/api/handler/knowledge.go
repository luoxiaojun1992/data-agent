package handler

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/luoxiaojun1992/data-agent/internal/service/knowledge"
)

// KnowledgeHandler provides HTTP handlers for knowledge base operations.
type KnowledgeHandler struct {
	svc     knowledge.KnowledgeService
	taskSvc task.TaskService // optional: nil when task queue is unavailable
}

// NewKnowledgeHandler creates a knowledge base handler.
func NewKnowledgeHandler(svc knowledge.KnowledgeService) *KnowledgeHandler {
	return &KnowledgeHandler{svc: svc}
}

// SetTaskService injects the task service for async KB indexing. Optional.
func (h *KnowledgeHandler) SetTaskService(ts task.TaskService) {
	h.taskSvc = ts
}

// UploadDoc creates a new knowledge document with optional file upload to GridFS.
func (h *KnowledgeHandler) UploadDoc(c *gin.Context) {
	userID, _ := c.Get("user_id")
	title := c.PostForm("title")
	fileName := c.PostForm("file_name")
	fileType := c.PostForm("file_type")
	sizeBytes := int64(0)
	if s := c.PostForm("size_bytes"); s != "" {
		_, _ = fmt.Sscanf(s, "%d", &sizeBytes)
	}

	// Upload file content to GridFS if a file was provided
	file, header, err := c.Request.FormFile("file")
	var gridFSFileID string
	if err == nil {
		defer file.Close()
		gridFSFileID, err = h.svc.UploadFile(fileName+"_"+header.Filename, header.Header.Get("Content-Type"), file)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "GridFS upload failed: " + err.Error()})
			return
		}
		if sizeBytes == 0 {
			sizeBytes = header.Size
		}
	}

	doc, err := h.svc.CreateDoc(userID.(string), title, fileName, fileType, sizeBytes, gridFSFileID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Enqueue async indexing via task queue (best-effort).
	if h.taskSvc != nil && gridFSFileID != "" {
		params := map[string]interface{}{
			"doc_id":         doc.ID,
			"gridfs_file_id": gridFSFileID,
		}
		if _, err := h.taskSvc.CreateTask("", userID.(string), task.TaskTypeKBIndex, nil, params, ""); err != nil {
			log.Printf("[kb] failed to enqueue index task for doc=%s: %v", doc.ID, err)
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

// ListDocs lists all documents for the current user.
func (h *KnowledgeHandler) ListDocs(c *gin.Context) {
	userID, _ := c.Get("user_id")
	docs, err := h.svc.ListDocs(userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, docs)
}

// Search performs hybrid search on the knowledge base.
func (h *KnowledgeHandler) Search(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' required"})
		return
	}
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	results, err := h.svc.Search(userID.(string), query, 5, role.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"query": query, "results": results})
}

// AddChunks adds semantic chunks to a document.
func (h *KnowledgeHandler) AddChunks(c *gin.Context) {
	docID := c.Param("id")
	var req struct {
		Chunks []string `json:"chunks"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.svc.AddChunks(docID, req.Chunks); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "indexed", "doc_id": docID, "chunk_count": len(req.Chunks)})
}

// ListAllDocs returns all knowledge documents globally (admin view).
func (h *KnowledgeHandler) ListAllDocs(c *gin.Context) {
	docs, err := h.svc.ListAllDocs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, docs)
}
