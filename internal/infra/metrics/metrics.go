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
	// Calendar bucket boundaries (day/week/month/year) follow loc (the caller's
	// timezone), while since/until remain absolute instants.
	Series(ctx context.Context, m Metric, since, until time.Time, gran Granularity, loc *time.Location) ([]Bucket, error)
}

// Granularity is a time-bucket granularity for trend series.
type Granularity string

const (
	GranularityHour  Granularity = "hour"
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

// BucketStart returns the start of the granularity bucket containing t in loc
// (exported wrapper; used by the dashboard handler for default windows).
func BucketStart(t time.Time, g Granularity, loc *time.Location) time.Time {
	return bucketStart(t, g, loc)
}

// bucketStart truncates t to the start of the given granularity bucket in loc.
// hour→hour, day→day, week→Monday, month→1st, year→Jan 1. It is the
// pure-function used by the reader's Go-side bucketing.
func bucketStart(t time.Time, g Granularity, loc *time.Location) time.Time {
	t = t.In(loc)
	switch g {
	case GranularityHour:
		return t.Truncate(time.Hour)
	case GranularityWeek:
		// Week starts on Monday (ISO).
		wd := int(t.Weekday())
		if wd == 0 {
			wd = 7
		}
		return time.Date(t.Year(), t.Month(), t.Day()-wd+1, 0, 0, 0, 0, loc)
	case GranularityMonth:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
	case GranularityYear:
		return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, loc)
	default: // day
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	}
}

// bucketAdvance returns the start of the next bucket after t for granularity g.
func bucketAdvance(t time.Time, g Granularity) time.Time {
	switch g {
	case GranularityHour:
		return t.Add(time.Hour)
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
// by the hour bucket start, it returns one Bucket per granularity step that
// overlaps [since, until), including the final (possibly partial) bucket that
// contains `until` — so a query ending mid-day still surfaces today's data.
// Calendar boundaries follow loc (the caller's timezone).
func bucketHours(hourSums map[time.Time]int64, since, until time.Time, g Granularity, loc *time.Location) []Bucket {
	start := bucketStart(since, g, loc)
	var out []Bucket
	for cur := start; cur.Before(until); cur = bucketAdvance(cur, g) {
		next := bucketAdvance(cur, g)
		var total int64
		for h, v := range hourSums {
			if !h.Before(cur) && h.Before(next) {
				total += v
			}
		}
		out = append(out, Bucket{Time: cur, Value: total})
	}
	return out
}

// ROI derives the output-per-10k-tokens ratio:
//
//	ROI = (artifact_created + task_completed) / (token_tokens / 10000)
//
// i.e. how many artifacts/tasks are produced per ten-thousand tokens. Tokens
// are counted in units of 10000 because raw token counts (millions) dwarf the
// artifact/task counts (tens), which would make the ratio converge to zero
// and carry no signal.
//
// When tokenToks is zero the result is 0 (never divide by zero).
func ROI(artifactCreated, taskCompleted, tokenToks int64) float64 {
	if tokenToks <= 0 {
		return 0
	}
	return float64(artifactCreated+taskCompleted) * 10000 / float64(tokenToks)
}
