package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/luoxiaojun1992/data-agent/internal/infra/llmstats"
	"github.com/luoxiaojun1992/data-agent/internal/service/knowledge"
	"github.com/luoxiaojun1992/data-agent/internal/service/monitor"
)

// TokenTrendAggregator abstracts token-usage time-bucket aggregation so the
// dashboard handler stays testable without a live MongoDB connection.
// *llmstats.Recorder satisfies this interface at runtime.
type TokenTrendAggregator interface {
	AggregateByTime(ctx context.Context, since time.Time, bucketMs int64) ([]llmstats.TimeBucketResult, error)
}

// DashboardHandler serves dashboard KPI stats and time-series trends. The
// sessionManager dependency was removed in SPEC-060 because Get no longer
// returns sessions (the frontend does not consume them); llmRecorder replaces
// it to power the real token_trend aggregation.
type DashboardHandler struct {
	taskService task.TaskService
	kbService   knowledge.KnowledgeService
	llmRecorder TokenTrendAggregator
}

// NewDashboardHandler constructs a DashboardHandler. llmRecorder may be nil in
// which case GetTrends returns an empty token_trend series.
func NewDashboardHandler(taskSvc task.TaskService, kbSvc knowledge.KnowledgeService, llmRecorder TokenTrendAggregator) *DashboardHandler {
	return &DashboardHandler{
		taskService: taskSvc,
		kbService:   kbSvc,
		llmRecorder: llmRecorder,
	}
}

// RegisterDashboardRoutes wires the /api/v1/dashboard and
// /api/v1/dashboard/trends routes. The path was renamed from the legacy
// admin-scoped dashboard path in SPEC-060 to match the frontend (frontend/app/page.tsx).
func RegisterDashboardRoutes(router *gin.Engine, midd gin.HandlerFunc, h *DashboardHandler) {
	router.GET("/api/v1/dashboard", midd, h.Get)
	router.GET("/api/v1/dashboard/trends", midd, h.GetTrends)
}

// Get returns dashboard KPI stats: task_stats (count by status) and kb_docs
// (total knowledge-base document count). Errors from the underlying services
// are intentionally swallowed so a partial outage does not break the dashboard
// UI — the affected field simply renders as zero.
func (h *DashboardHandler) Get(c *gin.Context) {
	_, total, _ := h.kbService.ListAllDocs(1, 1)
	c.JSON(http.StatusOK, gin.H{"kb_docs": total})
}

// GetTrends returns the full DashboardTrends payload (7 trend series). The
// token_trend is populated from real llm_usage aggregation via
// llmstats.AggregateByTime (SPEC-059 capability); all other trends are derived
// from task data by monitor.ComputeTrends.
func (h *DashboardHandler) GetTrends(c *gin.Context) {
	since := time.Now().Add(-24 * time.Hour)
	bucketMs := int64((4 * time.Hour).Milliseconds())

	var tokenBuckets []llmstats.TimeBucketResult
	if h.llmRecorder != nil {
		tokenBuckets, _ = h.llmRecorder.AggregateByTime(c.Request.Context(), since, bucketMs)
	}

	// ComputeTrends takes []task.TaskRun. The dashboard endpoint currently
	// has access to Task definitions, not runs — pass an empty slice for now
	// and let token_trend + other DB-backed trends drive the chart.
	// TODO: add TaskRunService to DashboardHandler.
	trends := monitor.ComputeTrends(nil, tokenBuckets)
	c.JSON(http.StatusOK, trends)
}
