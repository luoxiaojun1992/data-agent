package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apireview "github.com/luoxiaojun1992/data-agent/internal/service/apireview"
)

// APIReviewHandler provides HTTP handlers for API review.
type APIReviewHandler struct {
	svc *apireview.Service
}

// NewAPIReviewHandler creates an API review handler.
func NewAPIReviewHandler(svc *apireview.Service) *APIReviewHandler {
	return &APIReviewHandler{svc: svc}
}

// ListAPIReviews returns all API reviews.
func (h *APIReviewHandler) ListAPIReviews(c *gin.Context) {
	reviews, err := h.svc.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, reviews)
}

// CreateAPIReview submits a new API for review.
func (h *APIReviewHandler) CreateAPIReview(c *gin.Context) {
	var req struct {
		Name      string `json:"name" binding:"required"`
		FileName  string `json:"file_name" binding:"required"`
		Domain    string `json:"domain"`
		Version   string `json:"version"`
		Endpoints int    `json:"endpoints"`
		RateLimit int    `json:"rate_limit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少必填字段"})
		return
	}
	if req.Version == "" {
		req.Version = "3.0"
	}

	r, err := h.svc.Create(req.Name, req.FileName, req.Domain, req.Version, req.Endpoints, req.RateLimit, userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, r)
}

// ApproveAPIReview approves an API review.
func (h *APIReviewHandler) ApproveAPIReview(c *gin.Context) {
	reviewID := c.Param("id")
	if err := h.svc.Approve(reviewID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "approved"})
}

// RejectAPIReview rejects an API review.
func (h *APIReviewHandler) RejectAPIReview(c *gin.Context) {
	reviewID := c.Param("id")
	var req struct {
		Reason string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "驳回原因不能为空"})
		return
	}
	if err := h.svc.Reject(reviewID, req.Reason); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "rejected"})
}
