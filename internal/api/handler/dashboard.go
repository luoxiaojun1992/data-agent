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
	userID := c.GetString("user_id")

	tasks, _ := h.taskService.ListAllTasks(userID)
	docs, _ := h.kbService.ListAllDocs()

	c.JSON(http.StatusOK, gin.H{
		"task_stats": aggregateTaskStats(tasks),
		"kb_docs":    len(docs),
	})
}

// aggregateTaskStats counts tasks by status into the shape expected by the
// frontend (frontend/app/page.tsx L88): {total, pending, running, completed, failed}.
func aggregateTaskStats(tasks []*task.Task) map[string]int {
	stats := map[string]int{
		"total":     len(tasks),
		"pending":   0,
		"running":   0,
		"completed": 0,
		"failed":    0,
	}
	for _, t := range tasks {
		if t == nil {
			continue
		}
		switch t.Status {
		case task.StatusPending:
			stats["pending"]++
		case task.StatusRunning:
			stats["running"]++
		case task.StatusCompleted:
			stats["completed"]++
		case task.StatusFailed:
			stats["failed"]++
		}
	}
	return stats
}

// GetTrends returns the full DashboardTrends payload (7 trend series). The
// token_trend is populated from real llm_usage aggregation via
// llmstats.AggregateByTime (SPEC-059 capability); all other trends are derived
// from task data by monitor.ComputeTrends.
func (h *DashboardHandler) GetTrends(c *gin.Context) {
	userID := c.GetString("user_id")

	tasks, _ := h.taskService.ListAllTasks(userID)

	since := time.Now().Add(-24 * time.Hour)
	bucketMs := int64((4 * time.Hour).Milliseconds())

	var tokenBuckets []llmstats.TimeBucketResult
	if h.llmRecorder != nil {
		tokenBuckets, _ = h.llmRecorder.AggregateByTime(c.Request.Context(), since, bucketMs)
	}

	// ComputeTrends takes []task.Task (value slice); dereference the pointers
	// returned by the service. Nil entries are skipped to avoid a panic.
	taskValues := make([]task.Task, 0, len(tasks))
	for _, t := range tasks {
		if t == nil {
			continue
		}
		taskValues = append(taskValues, *t)
	}

	trends := monitor.ComputeTrends(taskValues, tokenBuckets)
	c.JSON(http.StatusOK, trends)
}
