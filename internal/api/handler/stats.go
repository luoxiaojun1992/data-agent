package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/infra/llmstats"
)

// StatsHandler exposes LLM token usage statistics. SPEC-059 introduces the
// /api/v1/stats/llm endpoint so the UI can verify that enhance calls are
// recorded in llm_usage (UI-160).
type StatsHandler struct {
	recorder *llmstats.Recorder
}

// NewStatsHandler constructs a StatsHandler backed by the given recorder.
func NewStatsHandler(rec *llmstats.Recorder) *StatsHandler {
	return &StatsHandler{recorder: rec}
}

// RegisterStatsRoutes mounts /api/v1/stats behind auth + admin guard.
func RegisterStatsRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *StatsHandler) {
	if h == nil {
		return
	}
	stats := router.Group("/api/v1/stats")
	stats.Use(jwt.AuthMiddleware(), middleware.RequirePermission(model.PermUserManage))
	stats.GET("/llm", h.GetLLMStats)
}

// llmStatsItem is the JSON-facing row for /stats/llm. It mirrors
// llmstats.AggregateResult plus a derived total_tokens (prompt+completion)
// so the response matches the SPEC-059 §4 contract.
type llmStatsItem struct {
	CallPoint        string `json:"call_point"`
	Count            int64  `json:"count"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
}

// GetLLMStats aggregates token usage by call_point.
// Query params: call_point (optional filter), since (optional RFC3339 lower bound).
// Requires admin permission (PermUserManage).
func (h *StatsHandler) GetLLMStats(c *gin.Context) {
	if h.recorder == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "llm stats recorder unavailable"})
		return
	}

	callPoint := c.Query("call_point")

	var since time.Time
	if raw := c.Query("since"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'since' format, expected RFC3339"})
			return
		}
		since = parsed
	}

	results, err := h.recorder.Aggregate(c.Request.Context(), callPoint, since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to aggregate llm token stats"})
		return
	}

	items := make([]llmStatsItem, 0, len(results))
	for _, r := range results {
		items = append(items, llmStatsItem{
			CallPoint:        r.CallPoint,
			Count:            r.Count,
			PromptTokens:     r.PromptTokens,
			CompletionTokens: r.CompletionTokens,
			TotalTokens:      r.PromptTokens + r.CompletionTokens,
		})
	}

	c.JSON(http.StatusOK, gin.H{"stats": items})
}
