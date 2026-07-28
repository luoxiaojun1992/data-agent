package monitor

import (
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/luoxiaojun1992/data-agent/internal/infra/llmstats"
)

// TrendPoint represents a single data point in a trend series.
type TrendPoint struct {
	Label string `json:"label"`
	Value int    `json:"value"`
}

type DashboardTrends struct {
	CallTrend     []TrendPoint `json:"call_trend"`
	ReqDist       []TrendPoint `json:"req_dist"`
	SuccessTrend  []TrendPoint `json:"success_trend"`
	TokenTrend    []TrendPoint `json:"token_trend"`
	DurationDist  []TrendPoint `json:"duration_dist"`
	OutputStats   []TrendPoint `json:"output_stats"`
	ROITrend      []TrendPoint `json:"roi_trend"`
	TaskStats     TaskStats    `json:"task_stats"`
}

type TaskStats struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Running   int64 `json:"running"`
	Completed int64 `json:"completed"`
	Failed    int64 `json:"failed"`
}

// ComputeTrends computes dashboard trends from real task run data plus real
// token statistics. When runs is nil, empty trends are returned.
func ComputeTrends(runs []task.TaskRun, tokenBuckets []llmstats.TimeBucketResult) *DashboardTrends {
	now := time.Now()
	t := &DashboardTrends{}
	t.TokenTrend = mapTokenBuckets(tokenBuckets, now)
	if runs != nil {
		t.CallTrend = computeCallTrend(runs, now)
		t.DurationDist = computeDurationDist(runs)
		t.ReqDist = t.CallTrend
		t.SuccessTrend = computeSuccessTrend(runs, now)
		t.OutputStats = computeOutputStats(runs)
		t.ROITrend = computeROITrend(runs, now)
	}
	return t
}

func mapTokenBuckets(buckets []llmstats.TimeBucketResult, now time.Time) []TrendPoint {
	hourBuckets := make([]int, 6)
	labels := []string{"0时", "4时", "8时", "12时", "16时", "20时"}
	bucketStarts := []int{0, 4, 8, 12, 16, 20}
	for _, b := range buckets {
		hour := b.BucketStart.Hour()
		for i, start := range bucketStarts {
			if hour >= start && hour < start+4 {
				hourBuckets[i] += int(b.TotalTokens)
				break
			}
		}
	}
	var trend []TrendPoint
	for i, count := range hourBuckets {
		trend = append(trend, TrendPoint{Label: labels[i], Value: count})
	}
	return trend
}

func computeCallTrend(runs []task.TaskRun, now time.Time) []TrendPoint {
	hourBuckets := make([]int, 6)
	labels := []string{"0时", "4时", "8时", "12时", "16时", "20时"}
	for _, r := range runs {
		diff := now.Sub(r.CreatedAt).Hours()
		if diff >= 24 { continue }
		bucketIdx := int(r.CreatedAt.Hour()) / 4
		if bucketIdx >= 0 && bucketIdx < 6 {
			hourBuckets[bucketIdx]++
		}
	}
	var trend []TrendPoint
	for i, count := range hourBuckets {
		trend = append(trend, TrendPoint{Label: labels[i], Value: count})
	}
	return trend
}

func computeDurationDist(runs []task.TaskRun) []TrendPoint {
	type bucket struct {
		label  string
		maxDur time.Duration
		count  int
	}
	buckets := []bucket{
		{"<5s", 5 * time.Second, 0},
		{"<30s", 30 * time.Second, 0},
		{"<1m", 1 * time.Minute, 0},
		{"<5m", 5 * time.Minute, 0},
		{">5m", 24 * time.Hour, 0},
	}
	for _, r := range runs {
		if r.CompletedAt == nil || r.DurationMs == 0 { continue }
		d := time.Duration(r.DurationMs) * time.Millisecond
		for i, b := range buckets {
			if d <= b.maxDur { buckets[i].count++; break }
		}
	}
	var trend []TrendPoint
	for _, b := range buckets {
		trend = append(trend, TrendPoint{Label: b.label, Value: b.count})
	}
	return trend
}

type completedFailed struct{ completed, failed int }

func computeSuccessTrend(runs []task.TaskRun, now time.Time) []TrendPoint {
	dayBuckets := make([]completedFailed, 7)
	dayLabels := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	for _, r := range runs {
		age := now.Sub(r.CreatedAt)
		if age > 7*24*time.Hour { continue }
		idx := int(r.CreatedAt.Weekday())
		if r.Status == task.StatusCompleted { dayBuckets[idx].completed++ }
		if r.Status == task.StatusFailed { dayBuckets[idx].failed++ }
	}
	var trend []TrendPoint
	for i, b := range dayBuckets {
		total := b.completed + b.failed
		if total == 0 {
			trend = append(trend, TrendPoint{Label: dayLabels[i], Value: 0})
			continue
		}
		trend = append(trend, TrendPoint{Label: dayLabels[i], Value: b.completed * 100 / total})
	}
	return trend
}

func computeOutputStats(runs []task.TaskRun) []TrendPoint {
	var withResult, noResult int
	for _, r := range runs {
		if len(r.Result) > 0 { withResult++ } else { noResult++ }
	}
	return []TrendPoint{
		{Label: "有产出", Value: withResult},
		{Label: "无产出", Value: noResult},
	}
}

func computeROITrend(runs []task.TaskRun, now time.Time) []TrendPoint {
	weekBuckets := make([]int, 4)
	labels := []string{"Week 1", "Week 2", "Week 3", "Week 4"}
	for _, r := range runs {
		if r.Status != task.StatusCompleted { continue }
		diff := now.Sub(r.CreatedAt).Hours()
		if diff >= 4*7*24 { continue }
		idx := int(diff / (7 * 24))
		if idx >= 0 && idx < 4 { weekBuckets[idx]++ }
	}
	var trend []TrendPoint
	for i, count := range weekBuckets {
		trend = append(trend, TrendPoint{Label: labels[i], Value: count})
	}
	return trend
}
