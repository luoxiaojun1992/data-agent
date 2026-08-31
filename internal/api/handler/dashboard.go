package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/infra/metrics"
	"github.com/luoxiaojun1992/data-agent/internal/service/knowledge"
)

// DashboardHandler serves dashboard KPI stats and time-series trends
// (SPEC-072). All counter metrics are read from the unified metrics.Reader
// (stats_hourly); kb_docs is a non-counter snapshot from the KB service.
type DashboardHandler struct {
	kbService knowledge.KnowledgeService
	reader    metrics.Reader
}

// NewDashboardHandler constructs a DashboardHandler. reader may be nil, in
// which case counter metrics render as zero.
func NewDashboardHandler(kbSvc knowledge.KnowledgeService, reader metrics.Reader) *DashboardHandler {
	return &DashboardHandler{kbService: kbSvc, reader: reader}
}

// RegisterDashboardRoutes wires the /api/v1/dashboard and
// /api/v1/dashboard/trends routes (JWT only — all logged-in users).
func RegisterDashboardRoutes(router *gin.Engine, midd gin.HandlerFunc, h *DashboardHandler) {
	router.GET("/api/v1/dashboard", midd, h.Get)
	router.GET("/api/v1/dashboard/trends", midd, h.GetTrends)
}

// summary is the JSON payload of GET /api/v1/dashboard.
type summary struct {
	KbDocs          int64   `json:"kb_docs"`
	TokenTokens     int64   `json:"token_tokens"`
	LLMCalls        int64   `json:"llm_calls"`
	APICalls        int64   `json:"api_calls"`
	ArtifactCreated int64   `json:"artifact_created"`
	TaskCompleted   int64   `json:"task_completed"`
	ROI             float64 `json:"roi"`
}

// Get returns the KPI summary (near-one-year cumulative). Partial failures
// are swallowed so a single outage does not break the dashboard.
func (h *DashboardHandler) Get(c *gin.Context) {
	ctx := c.Request.Context()
	since := time.Now().UTC().Add(-metrics.MaxRange)
	until := time.Now().UTC()

	s := summary{}
	_, s.KbDocs, _ = h.kbService.ListAllDocs(1, 1)
	if h.reader != nil {
		s.TokenTokens, _ = h.reader.Sum(ctx, metrics.MetricTokenTokens, since, until)
		s.LLMCalls, _ = h.reader.Sum(ctx, metrics.MetricLLMCalls, since, until)
		s.APICalls, _ = h.reader.Sum(ctx, metrics.MetricAPICalls, since, until)
		s.ArtifactCreated, _ = h.reader.Sum(ctx, metrics.MetricArtifact, since, until)
		s.TaskCompleted, _ = h.reader.Sum(ctx, metrics.MetricTaskCompleted, since, until)
	}
	s.ROI = metrics.ROI(s.ArtifactCreated, s.TaskCompleted, s.TokenTokens)
	c.JSON(http.StatusOK, s)
}

// trendPoint is one series data point with a time and value.
type trendPoint struct {
	Time  time.Time `json:"time"`
	Value int64     `json:"value"`
}

// roiPoint is one ROI series data point (value is a ratio, float64).
type roiPoint struct {
	Time  time.Time `json:"time"`
	Value float64   `json:"value"`
}

// trendsResponse is the JSON payload of GET /api/v1/dashboard/trends.
type trendsResponse struct {
	Granularity     string       `json:"granularity"` // window: day|week|month|year
	Bucket          string       `json:"bucket"`      // actual bucket size: hour|day|month
	TokenTokens     []trendPoint `json:"token_tokens"`
	LLMCalls        []trendPoint `json:"llm_calls"`
	APICalls        []trendPoint `json:"api_calls"`
	ArtifactCreated []trendPoint `json:"artifact_created"`
	TaskCompleted   []trendPoint `json:"task_completed"`
	ROI             []roiPoint   `json:"roi"`
}

// windowSpec maps a dashboard granularity (the display window) to the bucket
// size used inside that window and the default window length:
//
//	day   → 24-hour window, hour buckets
//	week  → 7-day window,   day buckets
//	month → 30-day window,  day buckets
//	year  → 12-month window, month buckets
type windowSpec struct {
	bucket metrics.Granularity
	window time.Duration
}

func windowFor(g metrics.Granularity) windowSpec {
	switch g {
	case metrics.GranularityWeek:
		return windowSpec{bucket: metrics.GranularityDay, window: 7 * 24 * time.Hour}
	case metrics.GranularityMonth:
		return windowSpec{bucket: metrics.GranularityDay, window: 30 * 24 * time.Hour}
	case metrics.GranularityYear:
		return windowSpec{bucket: metrics.GranularityMonth, window: 365 * 24 * time.Hour}
	default: // day
		return windowSpec{bucket: metrics.GranularityHour, window: 24 * time.Hour}
	}
}

// GetTrends returns per-metric time series. granularity selects the display
// window (day|week|month|year), and the bucket size inside that window is
// chosen automatically (hour for day, day for week/month, month for year).
// since/until query params are RFC3339 (UTC) and override the default window.
func (h *DashboardHandler) GetTrends(c *gin.Context) {
	ctx := c.Request.Context()
	gran := parseGranularity(c.Query("granularity"))
	spec := windowFor(gran)
	since, until, err := parseRange(c.Query("since"), c.Query("until"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	now := time.Now().UTC()
	if since.IsZero() {
		since = now.Add(-spec.window)
	}
	if until.IsZero() {
		until = now
	}

	resp := trendsResponse{Granularity: string(gran), Bucket: string(spec.bucket)}
	if h.reader == nil {
		c.JSON(http.StatusOK, resp)
		return
	}

	resp.TokenTokens = series(h.reader, ctx, metrics.MetricTokenTokens, since, until, spec.bucket)
	resp.LLMCalls = series(h.reader, ctx, metrics.MetricLLMCalls, since, until, spec.bucket)
	resp.APICalls = series(h.reader, ctx, metrics.MetricAPICalls, since, until, spec.bucket)
	resp.ArtifactCreated = series(h.reader, ctx, metrics.MetricArtifact, since, until, spec.bucket)
	resp.TaskCompleted = series(h.reader, ctx, metrics.MetricTaskCompleted, since, until, spec.bucket)
	resp.ROI = roiSeries(resp.ArtifactCreated, resp.TaskCompleted, resp.TokenTokens)

	c.JSON(http.StatusOK, resp)
}

// series fetches one metric's buckets and converts them to trend points.
func series(r metrics.Reader, ctx context.Context, m metrics.Metric, since, until time.Time, gran metrics.Granularity) []trendPoint {
	buckets, err := r.Series(ctx, m, since, until, gran)
	if err != nil {
		return nil
	}
	pts := make([]trendPoint, 0, len(buckets))
	for _, b := range buckets {
		pts = append(pts, trendPoint{Time: b.Time, Value: b.Value})
	}
	return pts
}

// roiSeries derives the ROI series from the artifact/task/token series (per
// bucket), using metrics.ROI for the token=0 boundary. All series share the
// same bucket boundaries, so they are aligned by index.
func roiSeries(artifact, task, token []trendPoint) []roiPoint {
	out := make([]roiPoint, 0, len(token))
	for i := range token {
		var a, tk int64
		if i < len(artifact) {
			a = artifact[i].Value
		}
		if i < len(task) {
			tk = task[i].Value
		}
		out = append(out, roiPoint{
			Time:  token[i].Time,
			Value: metrics.ROI(a, tk, token[i].Value),
		})
	}
	return out
}

func parseGranularity(raw string) metrics.Granularity {
	switch metrics.Granularity(raw) {
	case metrics.GranularityWeek, metrics.GranularityMonth, metrics.GranularityYear:
		return metrics.Granularity(raw)
	default:
		return metrics.GranularityDay
	}
}

func parseRange(sinceRaw, untilRaw string) (time.Time, time.Time, error) {
	var since, until time.Time
	var err error
	if sinceRaw != "" {
		if since, err = time.Parse(time.RFC3339, sinceRaw); err != nil {
			return since, until, err
		}
	}
	if untilRaw != "" {
		if until, err = time.Parse(time.RFC3339, untilRaw); err != nil {
			return since, until, err
		}
	}
	return since, until, nil
}
