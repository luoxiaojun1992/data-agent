package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/luoxiaojun1992/data-agent/internal/infra/metrics"
	kmocks "github.com/luoxiaojun1992/data-agent/internal/service/knowledge/mocks"
)

// fakeReader is a test double for metrics.Reader.
type fakeReader struct {
	sums   map[metrics.Metric]int64
	series map[metrics.Metric][]metrics.Bucket
}

func (f *fakeReader) Sum(_ context.Context, m metrics.Metric, _, _ time.Time) (int64, error) {
	return f.sums[m], nil
}

func (f *fakeReader) Series(_ context.Context, m metrics.Metric, _, _ time.Time, _ metrics.Granularity) ([]metrics.Bucket, error) {
	return f.series[m], nil
}

// capturingReader records the since/until of the last Sum call so tests can
// assert the KPI window is driven by granularity (not a hardcoded year).
type capturingReader struct {
	since, until time.Time
}

func (c *capturingReader) Sum(_ context.Context, _ metrics.Metric, since, until time.Time) (int64, error) {
	c.since, c.until = since, until
	return 0, nil
}

func (c *capturingReader) Series(_ context.Context, _ metrics.Metric, _, _ time.Time, _ metrics.Granularity) ([]metrics.Bucket, error) {
	return nil, nil
}

func newDashboardHandler(t *testing.T, sums map[metrics.Metric]int64, series map[metrics.Metric][]metrics.Bucket) *DashboardHandler {
	t.Helper()
	kb := kmocks.NewKnowledgeService(t)
	kb.On("ListAllDocs", 1, 1).Maybe().Return(nil, int64(42), nil)
	return NewDashboardHandler(kb, &fakeReader{sums: sums, series: series})
}

func TestDashboardGet(t *testing.T) {
	h := newDashboardHandler(t, map[metrics.Metric]int64{
		metrics.MetricTokenTokens:   100,
		metrics.MetricLLMCalls:      10,
		metrics.MetricAPICalls:      200,
		metrics.MetricArtifact:      5,
		metrics.MetricTaskCompleted: 15,
	}, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	h.Get(c)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, float64(42), body["kb_docs"])
	assert.Equal(t, float64(100), body["token_tokens"])
	assert.Equal(t, float64(10), body["llm_calls"])
	assert.Equal(t, float64(200), body["api_calls"])
	assert.Equal(t, float64(5), body["artifact_created"])
	assert.Equal(t, float64(15), body["task_completed"])
	// ROI = (5 + 15) * 10000 / 100 = 2000（每万 token 产出）
	assert.InDelta(t, 2000.0, body["roi"], 1e-9)
}

// TestDashboardGet_WindowFollowsGranularity verifies the KPI counters follow
// the selected time window (week → 7 days) while kb_docs stays the point-in-
// time total.
func TestDashboardGet_WindowFollowsGranularity(t *testing.T) {
	kb := kmocks.NewKnowledgeService(t)
	kb.On("ListAllDocs", 1, 1).Maybe().Return(nil, int64(42), nil)
	cr := &capturingReader{}
	h := NewDashboardHandler(kb, cr)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/dashboard?granularity=week", nil)
	h.Get(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.False(t, cr.since.IsZero(), "since should be set")
	require.False(t, cr.until.IsZero(), "until should be set")
	assert.InDelta(t, float64(7*24*time.Hour), float64(cr.until.Sub(cr.since)), float64(time.Second))

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	// kb_docs is a snapshot total and must not be windowed.
	assert.Equal(t, float64(42), body["kb_docs"])
}

func TestDashboardGet_InvalidSince(t *testing.T) {
	h := newDashboardHandler(t, nil, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/dashboard?since=not-a-date", nil)
	h.Get(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDashboardGet_ZeroTokensROI(t *testing.T) {
	h := newDashboardHandler(t, map[metrics.Metric]int64{
		metrics.MetricTokenTokens:   0,
		metrics.MetricArtifact:      5,
		metrics.MetricTaskCompleted: 3,
	}, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	h.Get(c)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, float64(0), body["roi"])
}

func TestDashboardGetTrends(t *testing.T) {
	series := map[metrics.Metric][]metrics.Bucket{
		metrics.MetricTokenTokens: {
			{Time: time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC), Value: 10},
			{Time: time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC), Value: 20},
		},
		metrics.MetricLLMCalls:      {{Time: time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC), Value: 1}},
		metrics.MetricAPICalls:      {{Time: time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC), Value: 5}},
		metrics.MetricArtifact:      {{Time: time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC), Value: 2}},
		metrics.MetricTaskCompleted: {{Time: time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC), Value: 3}},
	}
	h := newDashboardHandler(t, nil, series)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/trends?granularity=day", nil)
	h.GetTrends(c)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Granularity   string       `json:"granularity"`
		Bucket        string       `json:"bucket"`
		TokenTokens   []trendPoint `json:"token_tokens"`
		LLMCalls      []trendPoint `json:"llm_calls"`
		APICalls      []trendPoint `json:"api_calls"`
		TaskCompleted []trendPoint `json:"task_completed"`
		ROI           []roiPoint   `json:"roi"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "day", body.Granularity)
	assert.Equal(t, "hour", body.Bucket, "day window must use hour buckets")
	assert.Len(t, body.TokenTokens, 2)
	assert.Equal(t, int64(10), body.TokenTokens[0].Value)
	// ROI series aligned with token series (2 points).
	assert.Len(t, body.ROI, 2)
}

// TestWindowFor verifies the granularity → (bucket, window) mapping: the
// dashboard granularity is a display window, and the bucket size inside that
// window is chosen automatically.
func TestWindowFor(t *testing.T) {
	tests := []struct {
		gran   metrics.Granularity
		bucket metrics.Granularity
		window time.Duration
	}{
		{metrics.GranularityDay, metrics.GranularityHour, 24 * time.Hour},
		{metrics.GranularityWeek, metrics.GranularityDay, 7 * 24 * time.Hour},
		{metrics.GranularityMonth, metrics.GranularityDay, 30 * 24 * time.Hour},
		{metrics.GranularityYear, metrics.GranularityMonth, 365 * 24 * time.Hour},
	}
	for _, tc := range tests {
		spec := windowFor(tc.gran)
		assert.Equal(t, tc.bucket, spec.bucket, "granularity %s", tc.gran)
		assert.Equal(t, tc.window, spec.window, "granularity %s", tc.gran)
	}
}

func TestDashboardGetTrends_InvalidSince(t *testing.T) {
	h := newDashboardHandler(t, nil, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/trends?since=not-a-date", nil)
	h.GetTrends(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestParseGranularity(t *testing.T) {
	assert.Equal(t, metrics.GranularityDay, parseGranularity(""))
	assert.Equal(t, metrics.GranularityDay, parseGranularity("bogus"))
	assert.Equal(t, metrics.GranularityWeek, parseGranularity("week"))
	assert.Equal(t, metrics.GranularityMonth, parseGranularity("month"))
	assert.Equal(t, metrics.GranularityYear, parseGranularity("year"))
}

func TestParseRange(t *testing.T) {
	since, until, err := parseRange("", "")
	assert.NoError(t, err)
	assert.True(t, since.IsZero())
	assert.True(t, until.IsZero())

	since, until, err = parseRange("2026-08-01T00:00:00Z", "2026-08-31T00:00:00Z")
	assert.NoError(t, err)
	assert.Equal(t, "2026-08-01 00:00:00 +0000 UTC", since.String())
	assert.Equal(t, "2026-08-31 00:00:00 +0000 UTC", until.String())

	_, _, err = parseRange("bad", "")
	assert.Error(t, err)
}

func TestROISeries(t *testing.T) {
	artifact := []trendPoint{{Value: 5}, {Value: 0}}
	task := []trendPoint{{Value: 3}, {Value: 10}}
	token := []trendPoint{{Value: 100, Time: time.Now()}, {Value: 0, Time: time.Now().Add(time.Hour)}}

	out := roiSeries(artifact, task, token)
	require.Len(t, out, 2)
	// (5+3) * 10000 / 100 = 800（每万 token 产出）
	assert.InDelta(t, 800.0, out[0].Value, 1e-9)
	// token=0 → ROI 0
	assert.Equal(t, float64(0), out[1].Value)
}
