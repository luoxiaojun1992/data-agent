package monitor

import (
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
)

// TrendPoint represents a single data point in a trend series.
type TrendPoint struct {
	Label string `json:"label"`
	Value int    `json:"value"`
}

// DashboardTrends holds all time-series chart data.
type DashboardTrends struct {
	CallTrend    []TrendPoint `json:"call_trend"`    // 24h agent calls by hour
	DurationDist []TrendPoint `json:"duration_dist"` // task duration buckets
	ReqDist      []TrendPoint `json:"req_dist"`      // 24h request distribution
	SuccessTrend []TrendPoint `json:"success_trend"` // daily success rate
	TokenTrend   []TrendPoint `json:"token_trend"`   // token consumption trend
	OutputStats  []TrendPoint `json:"output_stats"`  // output artifacts by type
	ROITrend     []TrendPoint `json:"roi_trend"`     // weekly ROI
}

// ComputeTrends computes dashboard trends from real task data.
func ComputeTrends(tasks []task.Task, sessions []interface{}, docCount int) *DashboardTrends {
	now := time.Now()
	t := &DashboardTrends{}

	// 24h call trend — tasks by hour
	hourBuckets := make([]int, 6) // 4-hour buckets: 0,4,8,12,16,20
	hourLabels := []string{"0时", "4时", "8时", "12时", "16时", "20时"}
	nowHour := now.Hour()

	for _, task := range tasks {
		age := now.Sub(task.CreatedAt)
		if age > 24*time.Hour {
			continue
		}
		// Map task creation hour to nearest 4-hour bucket
		taskHour := task.CreatedAt.Hour()
		bucket := findClosestBucket(taskHour, []int{0, 4, 8, 12, 16, 20})
		if bucket >= 0 {
			hourBuckets[bucket]++
		}
	}
	_ = nowHour

	for i, count := range hourBuckets {
		t.CallTrend = append(t.CallTrend, TrendPoint{Label: hourLabels[i], Value: count})
	}

	// Duration distribution
	durBuckets := []struct {
		label  string
		maxDur time.Duration
		count  int
	}{
		{"<5s", 5 * time.Second, 0},
		{"<30s", 30 * time.Second, 0},
		{"<1m", 1 * time.Minute, 0},
		{"<5m", 5 * time.Minute, 0},
		{">5m", 24 * time.Hour, 0},
	}
	for _, task := range tasks {
		if task.CompletedAt == nil || task.DurationMs == 0 {
			continue
		}
		d := time.Duration(task.DurationMs) * time.Millisecond
		for i, b := range durBuckets {
			if d <= b.maxDur {
				durBuckets[i].count++
				break
			}
		}
	}
	for _, b := range durBuckets {
		t.DurationDist = append(t.DurationDist, TrendPoint{Label: b.label, Value: b.count})
	}

	// 24h request distribution — same as call trend
	t.ReqDist = t.CallTrend

	// Success rate trend — last 7 days
	dayBuckets := make([]completedFailed, 7)
	dayLabels := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	for _, task := range tasks {
		age := now.Sub(task.CreatedAt)
		if age > 7*24*time.Hour {
			continue
		}
		day := int(age.Hours()/24) % 7
		if string(task.Status) == "completed" {
			dayBuckets[day].completed++
		} else if string(task.Status) == "failed" {
			dayBuckets[day].failed++
		}
	}
	for i, b := range dayBuckets {
		total := b.completed + b.failed
		rate := 100
		if total > 0 {
			rate = (b.completed * 100) / total
		}
		t.SuccessTrend = append(t.SuccessTrend, TrendPoint{Label: dayLabels[i], Value: rate})
	}

	// Token trend — estimate from task count (real token tracking needs model integration)
	for i, p := range t.CallTrend {
		t.TokenTrend = append(t.TokenTrend, TrendPoint{Label: p.Label, Value: p.Value * 500})
	}

	// Output stats — count by task type/skill
	typeCount := make(map[string]int)
	for _, task := range tasks {
		for _, s := range task.SkillChain {
			typeCount[s]++
		}
	}
	for typ, count := range typeCount {
		t.OutputStats = append(t.OutputStats, TrendPoint{Label: typ, Value: count})
	}
	if len(t.OutputStats) == 0 {
		t.OutputStats = []TrendPoint{{Label: "report", Value: 0}, {Label: "sql", Value: 0}}
	}

	// ROI trend — correlate task count with output count (weekly)
	weeklyROI := make([]int, 4)
	labels := []string{"W1", "W2", "W3", "W4"}
	for _, task := range tasks {
		age := now.Sub(task.CreatedAt)
		if age > 28*24*time.Hour {
			continue
		}
		week := int(age.Hours()/(7*24)) % 4
		if string(task.Status) == "completed" {
			weeklyROI[week]++
		}
	}
	for i, c := range weeklyROI {
		t.ROITrend = append(t.ROITrend, TrendPoint{Label: labels[i], Value: c})
	}

	return t
}

type completedFailed struct {
	completed int
	failed    int
}

func findClosestBucket(hour int, buckets []int) int {
	best := -1
	bestDist := 100
	for i, b := range buckets {
		dist := abs(hour - b)
		if dist < bestDist {
			bestDist = dist
			best = i
		}
	}
	return best
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
