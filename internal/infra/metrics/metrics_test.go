package metrics

import (
	"sync"
	"testing"
	"time"
)

// ---- L1 pure functions ----

func TestROI(t *testing.T) {
	cases := []struct {
		name              string
		artifact, task, t int64
		want              float64
	}{
		{"normal", 10, 20, 100, 0.3},
		{"zero tokens", 10, 20, 0, 0},
		{"negative tokens", 10, 20, -5, 0},
		{"zero everything", 0, 0, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ROI(c.artifact, c.task, c.t)
			if got != c.want {
				t.Fatalf("ROI = %v, want %v", got, c.want)
			}
		})
	}
}

func TestHourBucket(t *testing.T) {
	at := time.Date(2026, 8, 31, 13, 45, 59, 123, time.UTC)
	got := HourBucket(at)
	want := time.Date(2026, 8, 31, 13, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("HourBucket = %v, want %v", got, want)
	}
}

func TestBucketStart(t *testing.T) {
	at := time.Date(2026, 8, 31, 13, 45, 0, 0, time.UTC) // Monday 2026-08-31
	cases := []struct {
		g    Granularity
		want time.Time
	}{
		{GranularityDay, time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)},
		{GranularityWeek, time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)}, // Monday
		{GranularityMonth, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)},
		{GranularityYear, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		if got := bucketStart(at, c.g); !got.Equal(c.want) {
			t.Errorf("bucketStart(%v) = %v, want %v", c.g, got, c.want)
		}
	}
	// A Sunday rolls back to the prior Monday.
	sun := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	if got := bucketStart(sun, GranularityWeek); !got.Equal(time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("week bucket of Sunday = %v, want Monday 2026-08-24", got)
	}
}

func TestBucketHours(t *testing.T) {
	d1 := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	// Hourly sums: 10 on Aug 31 00:00, 5 on Aug 31 23:00, 7 on Sep 1 00:00.
	hourSums := map[time.Time]int64{
		d1:                     10,
		d1.Add(23 * time.Hour): 5,
		d2:                     7,
	}
	buckets := bucketHours(hourSums, d1, d2.Add(24*time.Hour), GranularityDay)
	if len(buckets) != 2 {
		t.Fatalf("bucket count = %d, want 2", len(buckets))
	}
	if buckets[0].Value != 15 { // 10 + 5 both on Aug 31
		t.Errorf("Aug 31 value = %d, want 15", buckets[0].Value)
	}
	if buckets[1].Value != 7 {
		t.Errorf("Sep 1 value = %d, want 7", buckets[1].Value)
	}
}

func TestBucketHours_IncludesPartialFinalBucket(t *testing.T) {
	d1 := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	hourSums := map[time.Time]int64{
		d1:                10,
		d1.Add(23 * time.Hour): 5,
	}
	// until is mid-day on Sep 1 → the Sep 1 bucket (which holds no data here)
	// is still emitted as the final partial bucket.
	buckets := bucketHours(hourSums, d1, d1.Add(25*time.Hour), GranularityDay)
	if len(buckets) != 2 {
		t.Fatalf("bucket count = %d, want 2 (includes partial final day)", len(buckets))
	}
	if buckets[0].Value != 15 {
		t.Errorf("Aug 31 value = %d, want 15", buckets[0].Value)
	}
	if buckets[1].Value != 0 {
		t.Errorf("Sep 1 (partial) value = %d, want 0", buckets[1].Value)
	}
}

// ---- MongoCounter concurrency (white-box, no Mongo IO) ----

func newTestCounter() *MongoCounter {
	return &MongoCounter{
		buffer:        make(map[bucketKey]int64),
		flushInterval: 5 * time.Second,
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
	}
}

func TestMongoCounter_ConcurrentIncrNoLoss(t *testing.T) {
	c := newTestCounter()
	const goroutines = 50
	const perGoroutine = 100
	at := time.Date(2026, 8, 31, 13, 0, 0, 0, time.UTC)

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				_ = c.Incr(nil, MetricAPICalls, at, 1)
			}
		}()
	}
	wg.Wait()

	key := bucketKey{Metric: MetricAPICalls, Hour: HourBucket(at)}
	c.mu.Lock()
	got := c.buffer[key]
	c.mu.Unlock()
	if got != goroutines*perGoroutine {
		t.Fatalf("buffer value = %d, want %d (no lost update)", got, goroutines*perGoroutine)
	}
}

func TestMongoCounter_SwapDrainsBuffer(t *testing.T) {
	c := newTestCounter()
	at := time.Date(2026, 8, 31, 13, 0, 0, 0, time.UTC)
	_ = c.Incr(nil, MetricLLMCalls, at, 3)
	_ = c.Incr(nil, MetricLLMCalls, at, 2)

	old := c.swapBuffer()
	key := bucketKey{Metric: MetricLLMCalls, Hour: HourBucket(at)}
	if old[key] != 5 {
		t.Fatalf("swapped value = %d, want 5", old[key])
	}
	// Buffer is now empty.
	c.mu.Lock()
	remaining := len(c.buffer)
	c.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("buffer should be empty after swap, got %d entries", remaining)
	}
}

func TestMongoCounter_IncrZeroDelta(t *testing.T) {
	c := newTestCounter()
	at := time.Date(2026, 8, 31, 13, 0, 0, 0, time.UTC)
	_ = c.Incr(nil, MetricArtifact, at, 0)
	c.mu.Lock()
	n := len(c.buffer)
	c.mu.Unlock()
	if n != 0 {
		t.Fatalf("zero delta should not create a buffer entry, got %d", n)
	}
}

// ---- clampRange ----

func TestClampRange(t *testing.T) {
	now := time.Now().UTC()
	// since older than a year → clamped to now-MaxRange.
	old := now.Add(-2 * MaxRange)
	s, u := clampRange(old, now)
	if s.Before(now.Add(-MaxRange)) {
		t.Errorf("since not clamped: %v", s)
	}
	if !u.Equal(now) {
		t.Errorf("until = %v, want %v", u, now)
	}
	// Zero values default to a full year window.
	s, u = clampRange(time.Time{}, time.Time{})
	if s.IsZero() || u.IsZero() {
		t.Fatalf("zero range not defaulted: %v..%v", s, u)
	}
	span := u.Sub(s)
	if span < MaxRange-time.Second || span > MaxRange+time.Second {
		t.Errorf("default range span = %v, want ~%v", span, MaxRange)
	}
}
