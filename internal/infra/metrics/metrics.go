// Package metrics provides the unified statistics component (SPEC-072):
// a MongoDB-backed hourly counter (stats_hourly collection) plus a reader
// that aggregates hourly documents for dashboard KPIs and trends. All metrics
// are global system-level counts — no per-user dimension.
package metrics

import (
	"context"
	"time"
)

// Metric names the five counter metrics (ROI is derived, not stored).
type Metric string

const (
	MetricTokenTokens   Metric = "token_tokens"
	MetricLLMCalls      Metric = "llm_calls"
	MetricAPICalls      Metric = "api_calls"
	MetricArtifact      Metric = "artifact_created"
	MetricTaskCompleted Metric = "task_completed"
)

// Counter records counter increments. It is implemented by the buffered
// MongoDB counter (mongoCounter); call sites never touch the database
// directly and stay O(1) per increment.
type Counter interface {
	// Incr adds delta to the given metric at time at (bucketed to the hour).
	Incr(ctx context.Context, m Metric, at time.Time, delta int64) error
	// Stop flushes any buffered increments and stops the background flusher.
	Stop()
}

// Reader aggregates counter data for dashboards. All queries are global and
// bounded to at most one year.
type Reader interface {
	// Sum returns the total count of a metric over [since, until).
	Sum(ctx context.Context, m Metric, since, until time.Time) (int64, error)
	// Series returns the metric bucketed by granularity over [since, until).
	Series(ctx context.Context, m Metric, since, until time.Time, gran Granularity) ([]Bucket, error)
}

// Granularity is a time-bucket granularity for trend series.
type Granularity string

const (
	GranularityDay   Granularity = "day"
	GranularityWeek  Granularity = "week"
	GranularityMonth Granularity = "month"
	GranularityYear  Granularity = "year"
)

// Bucket is one aggregated data point of a series. Time is the bucket start
// (UTC).
type Bucket struct {
	Time  time.Time `json:"time"`
	Value int64     `json:"value"`
}

// MaxRange is the maximum queryable range (one year), enforced by the reader.
const MaxRange = 365 * 24 * time.Hour

// HourBucket truncates t to the hour boundary in UTC.
func HourBucket(t time.Time) time.Time {
	return t.UTC().Truncate(time.Hour)
}

// bucketStart truncates t to the start of the given granularity bucket (UTC).
// day→day, week→Monday, month→1st, year→Jan 1. It is the pure-function used
// by the reader's Go-side bucketing.
func bucketStart(t time.Time, g Granularity) time.Time {
	t = t.UTC()
	switch g {
	case GranularityWeek:
		// Week starts on Monday (ISO).
		wd := int(t.Weekday())
		if wd == 0 {
			wd = 7
		}
		return time.Date(t.Year(), t.Month(), t.Day()-wd+1, 0, 0, 0, 0, time.UTC)
	case GranularityMonth:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	case GranularityYear:
		return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	default: // day
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
}

// bucketAdvance returns the start of the next bucket after t for granularity g.
func bucketAdvance(t time.Time, g Granularity) time.Time {
	switch g {
	case GranularityWeek:
		return t.AddDate(0, 0, 7)
	case GranularityMonth:
		return t.AddDate(0, 1, 0)
	case GranularityYear:
		return t.AddDate(1, 0, 0)
	default: // day
		return t.AddDate(0, 0, 1)
	}
}

// bucketHours aggregates hourly documents into granularity buckets. It is the
// pure-function counterpart to the reader's Series: given per-hour sums keyed
// by the hour bucket start, it returns one Bucket per granularity step between
// since and until (empty buckets included).
func bucketHours(hourSums map[time.Time]int64, since, until time.Time, g Granularity) []Bucket {
	since = bucketStart(since, g)
	until = bucketStart(until, g)
	var out []Bucket
	for cur := since; cur.Before(until); cur = bucketAdvance(cur, g) {
		var total int64
		for h, v := range hourSums {
			if !h.Before(cur) && h.Before(bucketAdvance(cur, g)) {
				total += v
			}
		}
		out = append(out, Bucket{Time: cur, Value: total})
	}
	return out
}

// ROI derives the return-on-investment ratio:
//
//	ROI = (artifact_created + task_completed) / token_tokens
//
// When tokenToks is zero the result is 0 (never divide by zero).
func ROI(artifactCreated, taskCompleted, tokenToks int64) float64 {
	if tokenToks <= 0 {
		return 0
	}
	return float64(artifactCreated+taskCompleted) / float64(tokenToks)
}
