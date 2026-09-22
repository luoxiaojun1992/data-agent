package handler

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	auditsvc "github.com/luoxiaojun1992/data-agent/internal/service/audit"
)

// AuditHandler provides HTTP handlers for audit log operations.
type AuditHandler struct {
	svc auditsvc.AuditService
}

// NewAuditHandler creates an audit handler.
func NewAuditHandler(svc auditsvc.AuditService) *AuditHandler {
	return &AuditHandler{svc: svc}
}

// ListAuditLogs returns paginated audit logs with optional filters (SPEC-102).
// Query params: q (operator email), path (API path fuzzy), status_class
// (1xx~5xx), start/end (YYYY-MM-DD), page (≥1), page_size (1~100).
func (h *AuditHandler) ListAuditLogs(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	path := strings.TrimSpace(c.Query("path"))
	statusClass := c.Query("status_class")
	start := c.Query("start")
	end := c.Query("end")

	if err := validateAuditQueryStrings(q, path); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	page, pageSize, err := parseAuditPage(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, isSystemAdmin := taskIdentity(c)
	params := auditsvc.ListParams{
		Q:             q,
		Path:          path,
		StatusClass:   statusClass,
		Start:         start,
		End:           end,
		Page:          page,
		PageSize:      pageSize,
		ViewerUserID:  userID,
		IsSystemAdmin: isSystemAdmin,
	}

	result, err := h.svc.List(params)
	if err != nil {
		var ve *auditsvc.ValidationError
		if errors.As(err, &ve) {
			c.JSON(http.StatusBadRequest, gin.H{"error": ve.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// ExportAuditLogs exports audit logs as CSV (SPEC-102). Body shares the list
// filters (q/path/status_class/start/end) plus limit (1~50000) and format
// (only csv). The viewer/visibility filter is injected server-side.
func (h *AuditHandler) ExportAuditLogs(c *gin.Context) {
	var req auditsvc.ExportParams
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.Q = strings.TrimSpace(req.Q)
	req.Path = strings.TrimSpace(req.Path)
	if err := validateAuditQueryStrings(req.Q, req.Path); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Format != "csv" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid format: only csv is supported"})
		return
	}
	if req.Limit <= 0 || req.Limit > 50000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 50,000"})
		return
	}

	req.ViewerUserID, req.IsSystemAdmin = taskIdentity(c)

	logs, err := h.svc.Export(req)
	if err != nil {
		var ve *auditsvc.ValidationError
		if errors.As(err, &ve) {
			c.JSON(http.StatusBadRequest, gin.H{"error": ve.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("audit_logs_%s_%s.csv", req.Start, req.End)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	writer := csv.NewWriter(c.Writer)
	// BOM for Excel UTF-8
	_, _ = c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	_ = writer.Write([]string{"时间", "操作人", "操作类型", "详情", "IP", "状态码"})
	for _, log := range logs {
		_ = writer.Write([]string{
			log.CreatedAt.Format("2006-01-02 15:04:05"),
			log.UserID,
			log.ActionDesc,
			log.Details,
			log.IP,
			strconv.Itoa(log.StatusCode),
		})
	}
	writer.Flush()
}

// validateAuditQueryStrings enforces the q/path length limit (≤100, D6).
func validateAuditQueryStrings(q, path string) error {
	if len(q) > 100 {
		return fmt.Errorf("q too long: max 100 characters")
	}
	if len(path) > 100 {
		return fmt.Errorf("path too long: max 100 characters")
	}
	return nil
}

// parseAuditPage strictly parses page/page_size, rejecting non-integers and
// out-of-range values with 400 (SPEC-102 D6 — no silent clamping, unlike the
// shared parsePage used by other list pages).
func parseAuditPage(c *gin.Context) (page, pageSize int, err error) {
	page, err = strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		return 0, 0, fmt.Errorf("invalid page: must be a positive integer")
	}
	pageSize, err = strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || pageSize < 1 || pageSize > 100 {
		return 0, 0, fmt.Errorf("invalid page_size: must be an integer between 1 and 100")
	}
	return page, pageSize, nil
}
