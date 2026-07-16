package monitor

import (
	"testing"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
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
	trends := ComputeTrends(nil, nil, 0)
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

	trends := ComputeTrends(tasks, nil, 0)
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
	trends := ComputeTrends(nil, nil, 0)
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
