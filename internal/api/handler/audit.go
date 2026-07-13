package handler

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	auditsvc "github.com/luoxiaojun1992/data-agent/internal/service/audit"
)

// AuditHandler provides HTTP handlers for audit log operations.
type AuditHandler struct {
	svc *auditsvc.Service
}

// NewAuditHandler creates an audit handler.
func NewAuditHandler(svc *auditsvc.Service) *AuditHandler {
	return &AuditHandler{svc: svc}
}

// ListAuditLogs returns paginated audit logs with optional filters.
func (h *AuditHandler) ListAuditLogs(c *gin.Context) {
	skip, _ := strconv.ParseInt(c.DefaultQuery("skip", "0"), 10, 64)
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "20"), 10, 64)

	params := auditsvc.ListParams{
		Action: c.Query("action"),
		UserID: c.Query("user_id"),
		Start:  c.Query("start"),
		End:    c.Query("end"),
		Skip:   skip,
		Limit:  limit,
	}

	result, err := h.svc.List(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// ExportAuditLogs exports audit logs as CSV file download.
func (h *AuditHandler) ExportAuditLogs(c *gin.Context) {
	var req auditsvc.ExportParams
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Limit > 50000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "单次导出最多 50,000 条"})
		return
	}
	if req.Limit <= 0 {
		req.Limit = 5000
	}

	result, err := h.svc.List(auditsvc.ListParams{
		Action: req.Action,
		UserID: req.UserID,
		Start:  req.Start,
		End:    req.End,
		Limit:  req.Limit,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("audit_logs_%s_%s.csv", req.Start, req.End)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	writer := csv.NewWriter(c.Writer)
	// BOM for Excel UTF-8
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	writer.Write([]string{"时间", "操作人", "操作类型", "详情", "IP", "状态码"})
	for _, log := range result.Logs {
		writer.Write([]string{
			log.CreatedAt.Format("2006-01-02 15:04:05"),
			log.UserID,
			log.Action,
			log.Details,
			log.IP,
			strconv.Itoa(log.StatusCode),
		})
	}
	writer.Flush()
}
