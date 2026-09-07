package handler

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	kbdomain "github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
	"github.com/luoxiaojun1992/data-agent/internal/logic/webimport"
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
// (images), whichever is present. SPEC-081: the title is XSS-validated and
// truncated, and text/images are size-capped at the single source of truth.
func (h *KnowledgeHandler) UploadDoc(c *gin.Context) {
	userID, _ := c.Get("user_id")
	title := c.PostForm("title")
	fileName := c.PostForm("file_name")
	fileType := c.PostForm("file_type")
	sizeBytes := int64(0)
	if s := c.PostForm("size_bytes"); s != "" {
		_, _ = fmt.Sscanf(s, "%d", &sizeBytes)
	}

	// SPEC-081 §4.4: title is a plain-text label that never enters the LLM —
	// XSS-block it at the handler entry (no escaping, no mutation).
	if err := security.ValidateXSS(title); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "标题包含非法内容"})
		return
	}
	title = truncateTitleRunes(title, kbdomain.MaxKBTitleRunes)

	ctx := c.Request.Context()

	// Path 1: multipart file upload (text documents) → shared CreateFromText.
	if file, _, err := c.Request.FormFile("file"); err == nil {
		defer file.Close()
		data, rErr := io.ReadAll(file)
		if rErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "read file failed: " + rErr.Error()})
			return
		}
		if len(data) > kbdomain.MaxKBTextBytes {
			c.JSON(http.StatusBadRequest, gin.H{"error": "文本超过 5MB 上限"})
			return
		}
		doc, err := h.svc.CreateFromText(ctx, userID.(string), title, fileName, string(data))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, doc)
		return
	}

	// Path 2: base64 image upload (data URI prefix optional) → shared CreateFromImage.
	if base64Data := c.PostForm("file_base64"); base64Data != "" {
		if idx := strings.Index(base64Data, ";base64,"); idx >= 0 {
			base64Data = base64Data[idx+len(";base64,"):]
		}
		decoded, derr := base64.StdEncoding.DecodeString(base64Data)
		if derr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid base64 image: " + derr.Error()})
			return
		}
		if len(decoded) > kbdomain.MaxKBImageBytes {
			c.JSON(http.StatusBadRequest, gin.H{"error": "图片超过 1MB 上限"})
			return
		}
		mimeType := c.PostForm("mime_type")
		if mimeType == "" {
			mimeType = "image/png"
		}
		doc, err := h.svc.CreateFromImage(ctx, userID.(string), title, fileName, decoded, mimeType)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, doc)
		return
	}

	// Metadata-only (no file content): legacy backward-compat path.
	doc, err := h.svc.CreateDoc(userID.(string), title, fileName, fileType, sizeBytes, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, doc)
}

// ImportURL parses a web page URL and creates KB docs from its rendered text
// and images (SPEC-081). The URL is fetched server-side (headless chrome).
func (h *KnowledgeHandler) ImportURL(c *gin.Context) {
	var req struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID, _ := c.Get("user_id")
	result, err := h.svc.ImportURL(c.Request.Context(), userID.(string), req.URL)
	if err != nil {
		c.JSON(importURLErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// importURLErrorStatus maps a webimport sentinel error to its HTTP status
// (SPEC-081 §4.1).
func importURLErrorStatus(err error) int {
	switch {
	case errors.Is(err, webimport.ErrInvalidURL):
		return http.StatusBadRequest
	case errors.Is(err, webimport.ErrSSRFBlocked):
		return http.StatusForbidden
	case errors.Is(err, webimport.ErrNoContent):
		return http.StatusUnprocessableEntity
	case errors.Is(err, webimport.ErrRenderFailed):
		return http.StatusBadGateway
	case errors.Is(err, webimport.ErrRenderUnavailable):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// truncateTitleRunes truncates a title to at most n runes without splitting a
// UTF-8 sequence (SPEC-081 §5.3 — title overflow truncates, never rejects).
func truncateTitleRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// GetDoc retrieves a knowledge document (ownership-checked).
func (h *KnowledgeHandler) GetDoc(c *gin.Context) {
	docID := c.Param("id")
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")
	isSystemAdmin := role == "system_admin"
	doc, err := h.svc.GetDoc(docID, userID.(string), isSystemAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, doc)
}

// DeleteDoc removes a knowledge document and its chunks (cascade, ownership-checked).
func (h *KnowledgeHandler) DeleteDoc(c *gin.Context) {
	docID := c.Param("id")
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")
	isSystemAdmin := role == "system_admin"
	if err := h.svc.DeleteDoc(docID, userID.(string), isSystemAdmin); err != nil {
		if errors.Is(err, knowledge.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted", "id": docID})
}

// ListDocs lists documents visible to the current user.
// System admin: all docs. Regular user: own docs + public docs.
// q filters by title/file_name at the DB layer (SPEC-075).
func (h *KnowledgeHandler) ListDocs(c *gin.Context) {
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")
	q := c.Query("q")
	page, pageSize := parsePage(c)
	isSystemAdmin := role == "system_admin"
	docs, total, err := h.svc.ListDocsByVisibility(userID.(string), isSystemAdmin, q, page, pageSize)
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

// SetPublicFlag toggles the is_public flag on a knowledge document (ownership-checked).
func (h *KnowledgeHandler) SetPublicFlag(c *gin.Context) {
	docID := c.Param("id")
	var req struct {
		IsPublic bool `json:"is_public"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")
	isSystemAdmin := role == "system_admin"
	if err := h.svc.SetPublicFlag(c.Request.Context(), docID, req.IsPublic, userID.(string), isSystemAdmin); err != nil {
		if errors.Is(err, knowledge.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
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
