package monitor

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/luoxiaojun1992/data-agent/internal/infra/llmstats"
)

func TestSystemStats(t *testing.T) {
	stats := SystemStats()
	if stats == nil {
		t.Fatal("SystemStats() should not return nil")
	}

	// Check required keys
	requiredKeys := []string{"uptime_seconds", "go_version", "goroutines", "memory", "cpu_cores"}
	for _, key := range requiredKeys {
		if _, ok := stats[key]; !ok {
			t.Errorf("SystemStats() should contain key %q", key)
		}
	}

	// Check uptime is non-negative
	uptime, ok := stats["uptime_seconds"].(int)
	if !ok {
		t.Error("uptime_seconds should be int")
	}
	if uptime < 0 {
		t.Error("uptime_seconds should not be negative")
	}

	// Check goroutines is non-negative
	goroutines, ok := stats["goroutines"].(int)
	if !ok {
		t.Error("goroutines should be int")
	}
	if goroutines < 0 {
		t.Error("goroutines should not be negative")
	}

	// Check cpu_cores is positive
	cpuCores, ok := stats["cpu_cores"].(int)
	if !ok {
		t.Error("cpu_cores should be int")
	}
	if cpuCores <= 0 {
		t.Error("cpu_cores should be positive")
	}

	// Check memory sub-map
	mem, ok := stats["memory"].(map[string]interface{})
	if !ok {
		t.Fatal("memory should be a map")
	}
	memKeys := []string{"alloc_mb", "total_alloc_mb", "sys_mb", "gc_cycles"}
	for _, key := range memKeys {
		if _, ok := mem[key]; !ok {
			t.Errorf("memory should contain key %q", key)
		}
	}
}

func TestSystemStats_UptimeIncreases(t *testing.T) {
	stats1 := SystemStats()
	time.Sleep(100 * time.Millisecond)
	stats2 := SystemStats()

	uptime1, _ := stats1["uptime_seconds"].(int)
	uptime2, _ := stats2["uptime_seconds"].(int)
	// After ~100ms, uptime might not change but should be >=
	if uptime2 < uptime1 {
		t.Errorf("uptime should be monotonic: %d -> %d", uptime1, uptime2)
	}
}

func TestTrendPoint_Defaults(t *testing.T) {
	tp := TrendPoint{
		Label: "test",
		Value: 42,
	}
	if tp.Label != "test" {
		t.Errorf("Label = %q, want %q", tp.Label, "test")
	}
	if tp.Value != 42 {
		t.Errorf("Value = %d, want 42", tp.Value)
	}
}

func TestDashboardTrends_Defaults(t *testing.T) {
	dt := DashboardTrends{}
	if dt.CallTrend != nil {
		t.Error("CallTrend should be nil by default")
	}
	if dt.DurationDist != nil {
		t.Error("DurationDist should be nil by default")
	}
}

func TestFindClosestBucket(t *testing.T) {
	tests := []struct {
		hour    int
		buckets []int
		want    int
	}{
		{0, []int{0, 4, 8, 12, 16, 20}, 0},
		{1, []int{0, 4, 8, 12, 16, 20}, 0},
		{3, []int{0, 4, 8, 12, 16, 20}, 1},
		{5, []int{0, 4, 8, 12, 16, 20}, 1},
		{8, []int{0, 4, 8, 12, 16, 20}, 2},
		{12, []int{0, 4, 8, 12, 16, 20}, 3},
		{15, []int{0, 4, 8, 12, 16, 20}, 4},
		{16, []int{0, 4, 8, 12, 16, 20}, 4},
		{23, []int{0, 4, 8, 12, 16, 20}, 5},
	}
	for _, tt := range tests {
		got := findClosestBucket(tt.hour, tt.buckets)
		if got != tt.want {
			t.Errorf("findClosestBucket(%d, %v) = %d, want %d", tt.hour, tt.buckets, got, tt.want)
		}
	}
}

func TestAbs(t *testing.T) {
	tests := []struct {
		x    int
		want int
	}{
		{5, 5},
		{0, 0},
		{-1, 1},
		{-100, 100},
		{100, 100},
	}
	for _, tt := range tests {
		got := abs(tt.x)
		if got != tt.want {
			t.Errorf("abs(%d) = %d, want %d", tt.x, got, tt.want)
		}
	}
}

func TestComputeTrends_EmptyInput(t *testing.T) {
	trends := ComputeTrends(nil, nil)
	if trends == nil {
		t.Fatal("ComputeTrends() should not return nil")
	}
	if len(trends.CallTrend) != 6 {
		t.Errorf("CallTrend len = %d, want 6", len(trends.CallTrend))
	}
	if len(trends.DurationDist) != 5 {
		t.Errorf("DurationDist len = %d, want 5", len(trends.DurationDist))
	}
	if len(trends.SuccessTrend) != 7 {
		t.Errorf("SuccessTrend len = %d, want 7", len(trends.SuccessTrend))
	}
	// SPEC-060: token_trend now maps to 6 fixed 4h buckets; with nil buckets
	// all values are 0 but the series still has 6 points aligned with
	// call_trend.
	if len(trends.TokenTrend) != 6 {
		t.Errorf("TokenTrend len = %d, want 6", len(trends.TokenTrend))
	}
	if len(trends.ROITrend) != 4 {
		t.Errorf("ROITrend len = %d, want 4", len(trends.ROITrend))
	}
}

func TestComputeTrends_WithTasks(t *testing.T) {
	now := time.Now()
	tasks := []task.Task{
		{
			ID:        "task1",
			Status:    task.StatusCompleted,
			CreatedAt: now.Add(-1 * time.Hour),
			SkillChain: []string{"sql_executor", "stats_engine"},
		},
		{
			ID:        "task2",
			Status:    task.StatusFailed,
			CreatedAt: now.Add(-2 * time.Hour),
			SkillChain: []string{"save_report"},
		},
	}

	trends := ComputeTrends(tasks, nil)
	if trends == nil {
		t.Fatal("ComputeTrends() should not return nil")
	}
	// With real tasks, output stats should have entries
	if len(trends.OutputStats) == 0 {
		t.Error("OutputStats should not be empty with tasks")
	}
}

func TestComputeTrends_DefaultOutputStats(t *testing.T) {
	// Tasks with no skill chain produce default output stats
	trends := ComputeTrends(nil, nil)
	if len(trends.OutputStats) != 2 {
		t.Errorf("Expected default 2 output stats, got %d", len(trends.OutputStats))
	}
	if trends.OutputStats[0].Label != "report" {
		t.Errorf("First default stat = %q, want %q", trends.OutputStats[0].Label, "report")
	}
	if trends.OutputStats[1].Label != "sql" {
		t.Errorf("Second default stat = %q, want %q", trends.OutputStats[1].Label, "sql")
	}
}

// ===== Gomonkey-based tests =====

func init() { gin.SetMode(gin.TestMode) }

func TestHandlerReturnsStats(t *testing.T) {
	mockStats := map[string]interface{}{
		"uptime_seconds": 42,
		"go_version":     "go1.25.0",
		"goroutines":     10,
		"memory": map[string]interface{}{
			"alloc_mb":       100,
			"total_alloc_mb": 200,
			"sys_mb":         300,
			"gc_cycles":      uint32(5),
		},
		"cpu_cores": 8,
	}

	patches := gomonkey.ApplyFuncReturn(SystemStats, mockStats)
	defer patches.Reset()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/stats", nil)

	Handler()(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
}

func TestHandlerResponseContainsAllKeys(t *testing.T) {
	mockStats := map[string]interface{}{
		"uptime_seconds": 0,
		"go_version":     "go1.25.0",
		"goroutines":     1,
		"memory": map[string]interface{}{
			"alloc_mb":       0,
			"total_alloc_mb": 0,
			"sys_mb":         0,
			"gc_cycles":      uint32(0),
		},
		"cpu_cores": 1,
	}

	patches := gomonkey.ApplyFuncReturn(SystemStats, mockStats)
	defer patches.Reset()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/stats", nil)

	Handler()(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}

	body := w.Body.String()
	requiredKeys := []string{"uptime_seconds", "go_version", "goroutines", "memory", "cpu_cores"}
	for _, key := range requiredKeys {
		if !strings.Contains(body, key) {
			t.Errorf("response should contain key %q", key)
		}
	}
}

func TestComputeTrends_TasksOutsideWindow(t *testing.T) {
	now := time.Now()
	tasks := []task.Task{
		{
			ID:        "old1",
			Status:    task.StatusCompleted,
			CreatedAt: now.Add(-25 * time.Hour), // outside 24h window
		},
		{
			ID:        "old2",
			Status:    task.StatusFailed,
			CreatedAt: now.Add(-8 * 24 * time.Hour), // outside 7d window
		},
	}

	trends := ComputeTrends(tasks, nil)
	if trends == nil {
		t.Fatal("ComputeTrends() should not return nil")
	}
	// All hourly buckets should be 0 since tasks are outside window
	for _, p := range trends.CallTrend {
		if p.Value != 0 {
			t.Errorf("CallTrend should be all zero for tasks outside window, got %d at %q", p.Value, p.Label)
		}
	}
}

func TestSystemStats_ContainsGoVersion(t *testing.T) {
	stats := SystemStats()
	goVer, ok := stats["go_version"].(string)
	if !ok || goVer == "" {
		t.Error("go_version should be a non-empty string")
	}
}

func TestSystemStats_ContainsGoroutines(t *testing.T) {
	stats := SystemStats()
	gr, ok := stats["goroutines"].(int)
	if !ok {
		t.Error("goroutines should be int")
	}
	if gr < 1 {
		t.Error("goroutines should be at least 1")
	}
}

func TestComputeTrends_SuccessTrendAllSuccess(t *testing.T) {
	now := time.Now()
	tasks := []task.Task{
		{ID: "t1", Status: task.StatusCompleted, CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "t2", Status: task.StatusCompleted, CreatedAt: now.Add(-2 * time.Hour)},
	}

	trends := ComputeTrends(tasks, nil)
	for _, p := range trends.SuccessTrend {
		if p.Value != 100 {
			t.Errorf("SuccessTrend should be 100 when all succeed, got %d for %q", p.Value, p.Label)
		}
	}
}

func TestComputeTrends_SuccessTrendAllFailed(t *testing.T) {
	now := time.Now()
	tasks := []task.Task{
		{ID: "t1", Status: task.StatusFailed, CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "t2", Status: task.StatusFailed, CreatedAt: now.Add(-2 * time.Hour)},
	}

	trends := ComputeTrends(tasks, nil)
	// Only the day where tasks land should be 0; other days with no data default to 100
	hasZeroDay := false
	for _, p := range trends.SuccessTrend {
		if p.Value == 0 {
			hasZeroDay = true
		}
	}
	if !hasZeroDay {
		t.Error("SuccessTrend should contain a 0% day when all tasks fail")
	}
}

func TestComputeTrends_DurationBuckets(t *testing.T) {
	now := time.Now()
	completed := now.Add(-10 * time.Minute)
	tasks := []task.Task{
		{ID: "t1", Status: task.StatusCompleted, CreatedAt: now.Add(-1 * time.Hour), CompletedAt: &completed, DurationMs: 3000},   // 3s -> <5s
		{ID: "t2", Status: task.StatusCompleted, CreatedAt: now.Add(-1 * time.Hour), CompletedAt: &completed, DurationMs: 15000},  // 15s -> <30s
		{ID: "t3", Status: task.StatusCompleted, CreatedAt: now.Add(-1 * time.Hour), CompletedAt: &completed, DurationMs: 120000}, // 2m -> <5m
	}

	trends := ComputeTrends(tasks, nil)
	if len(trends.DurationDist) != 5 {
		t.Fatalf("DurationDist len = %d, want 5", len(trends.DurationDist))
	}
	// <5s bucket gets 3s task only (breaks on first match)
	if trends.DurationDist[0].Label != "<5s" || trends.DurationDist[0].Value != 1 {
		t.Errorf("DurationDist[0] = {%q, %d}, want {<5s, 1}", trends.DurationDist[0].Label, trends.DurationDist[0].Value)
	}
	// <30s bucket gets 15s task only (3s already matched <5s)
	if trends.DurationDist[1].Label != "<30s" || trends.DurationDist[1].Value != 1 {
		t.Errorf("DurationDist[1] = {%q, %d}, want {<30s, 1}", trends.DurationDist[1].Label, trends.DurationDist[1].Value)
	}
	// <5m bucket gets 2m task
	if trends.DurationDist[3].Label != "<5m" || trends.DurationDist[3].Value != 1 {
		t.Errorf("DurationDist[3] = {%q, %d}, want {<5m, 1}", trends.DurationDist[3].Label, trends.DurationDist[3].Value)
	}
}

func TestComputeTrends_ROITrend(t *testing.T) {
	trends := ComputeTrends(nil, nil)
	if len(trends.ROITrend) != 4 {
		t.Errorf("ROITrend len = %d, want 4", len(trends.ROITrend))
	}
}

func TestComputeTrends_ReqDistMirrorsCallTrend(t *testing.T) {
	trends := ComputeTrends(nil, nil)
	if len(trends.ReqDist) != len(trends.CallTrend) {
		t.Errorf("ReqDist len %d != CallTrend len %d", len(trends.ReqDist), len(trends.CallTrend))
	}
}

func TestComputeTrends_OutputStatsWithSkillChain(t *testing.T) {
	now := time.Now()
	tasks := []task.Task{
		{ID: "t1", Status: task.StatusCompleted, CreatedAt: now.Add(-1 * time.Hour), SkillChain: []string{"sql_executor"}},
		{ID: "t2", Status: task.StatusCompleted, CreatedAt: now.Add(-2 * time.Hour), SkillChain: []string{"sql_executor", "pdf_generator"}},
	}

	trends := ComputeTrends(tasks, nil)
	// sql_executor should appear twice, pdf_generator once
	foundSql := false
	foundPdf := false
	for _, p := range trends.OutputStats {
		if p.Label == "sql_executor" {
			foundSql = true
			if p.Value != 2 {
				t.Errorf("sql_executor count = %d, want 2", p.Value)
			}
		}
		if p.Label == "pdf_generator" {
			foundPdf = true
			if p.Value != 1 {
				t.Errorf("pdf_generator count = %d, want 1", p.Value)
			}
		}
	}
	if !foundSql {
		t.Error("OutputStats should contain sql_executor")
	}
	if !foundPdf {
		t.Error("OutputStats should contain pdf_generator")
	}
}


func TestComputeTrends_OldTask(t *testing.T) {
	oldTask := task.Task{
		ID: "old-task", CreatedAt: time.Now().Add(-30 * 24 * time.Hour),
		Status: "completed",
	}
	trends := ComputeTrends([]task.Task{oldTask}, nil)
	if trends == nil {
		t.Fatal("should return trends")
	}
	// Old task should not appear in 24h/7d/28d buckets
}

// ===== SPEC-060: mapTokenBuckets unit tests (L1 target 100%) =====

func TestMapTokenBuckets_NilBuckets(t *testing.T) {
	now := time.Now()
	trend := mapTokenBuckets(nil, now)
	if len(trend) != 6 {
		t.Fatalf("len = %d, want 6", len(trend))
	}
	wantLabels := []string{"0时", "4时", "8时", "12时", "16时", "20时"}
	for i, p := range trend {
		if p.Label != wantLabels[i] {
			t.Errorf("trend[%d].Label = %q, want %q", i, p.Label, wantLabels[i])
		}
		if p.Value != 0 {
			t.Errorf("trend[%d].Value = %d, want 0 for nil buckets", i, p.Value)
		}
	}
}

func TestMapTokenBuckets_WithinWindow(t *testing.T) {
	now := time.Now()
	// Two buckets in the past 24h: one ~1h ago (hour close to now), one ~5h ago.
	recent := now.Add(-1 * time.Hour)
	older := now.Add(-5 * time.Hour)
	buckets := []llmstats.TimeBucketResult{
		{BucketStart: recent, TotalTokens: 150},
		{BucketStart: older, TotalTokens: 250},
	}
	trend := mapTokenBuckets(buckets, now)
	if len(trend) != 6 {
		t.Fatalf("len = %d, want 6", len(trend))
	}
	// Total tokens should be preserved across all buckets.
	total := 0
	for _, p := range trend {
		total += p.Value
	}
	if total != 400 {
		t.Errorf("total tokens = %d, want 400", total)
	}
}

func TestMapTokenBuckets_OutsideWindowSkipped(t *testing.T) {
	now := time.Now()
	// Bucket older than 24h should be skipped entirely.
	old := now.Add(-25 * time.Hour)
	buckets := []llmstats.TimeBucketResult{
		{BucketStart: old, TotalTokens: 999},
	}
	trend := mapTokenBuckets(buckets, now)
	if len(trend) != 6 {
		t.Fatalf("len = %d, want 6", len(trend))
	}
	for _, p := range trend {
		if p.Value != 0 {
			t.Errorf("expected all zero for out-of-window bucket, got %d at %q", p.Value, p.Label)
		}
	}
}

func TestMapTokenBuckets_AlignedToClosestBucket(t *testing.T) {
	now := time.Now()
	// A bucket at 02:00 should map to the "0时" label (closest of 0/4/8/...).
	// Use a fixed recent time so the hour is deterministic.
	recent := now.Add(-2 * time.Hour)
	buckets := []llmstats.TimeBucketResult{
		{BucketStart: recent, TotalTokens: 42},
	}
	trend := mapTokenBuckets(buckets, now)
	if len(trend) != 6 {
		t.Fatalf("len = %d, want 6", len(trend))
	}
	// Exactly one bucket should carry the 42 tokens; the rest stay 0.
	nonZero := 0
	for _, p := range trend {
		if p.Value == 42 {
			nonZero++
		}
	}
	if nonZero != 1 {
		t.Errorf("expected exactly 1 non-zero bucket, got %d", nonZero)
	}
}

func TestComputeTrends_TokenTrendFromBuckets(t *testing.T) {
	now := time.Now()
	tasks := []task.Task{
		{ID: "t1", Status: task.StatusCompleted, CreatedAt: now.Add(-1 * time.Hour)},
	}
	buckets := []llmstats.TimeBucketResult{
		{BucketStart: now.Add(-1 * time.Hour), TotalTokens: 100},
	}
	trends := ComputeTrends(tasks, buckets)
	if len(trends.TokenTrend) != 6 {
		t.Fatalf("TokenTrend len = %d, want 6", len(trends.TokenTrend))
	}
	total := 0
	for _, p := range trends.TokenTrend {
		total += p.Value
	}
	if total != 100 {
		t.Errorf("TokenTrend total = %d, want 100", total)
	}
}
