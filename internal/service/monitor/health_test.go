package monitor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func okProbe(_ context.Context) error { return nil }

func failProbe(_ context.Context) error { return errors.New("connection refused") }

// slowProbe blocks until the probe context is cancelled, mimicking a hung
// dependency that only the per-probe timeout can terminate.
func slowProbe(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestHealthService_AllUp_StatusOK(t *testing.T) {
	svc := NewHealthService("v1.5.0",
		Probe{Name: "mongodb", Check: okProbe},
		Probe{Name: "redis", Check: okProbe},
	)
	resp := svc.Check(context.Background())

	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok", resp.Status)
	}
	if resp.Version != "v1.5.0" {
		t.Errorf("version = %q, want v1.5.0", resp.Version)
	}
	if resp.LatencyMS < 0 {
		t.Errorf("latency_ms = %d, want >= 0", resp.LatencyMS)
	}
	if resp.UptimeSec < 0 {
		t.Errorf("uptime_seconds = %d, want >= 0", resp.UptimeSec)
	}
	if got := resp.Dependencies["mongodb"].Status; got != "up" {
		t.Errorf("mongodb status = %q, want up", got)
	}
	if got := resp.Dependencies["redis"].Status; got != "up" {
		t.Errorf("redis status = %q, want up", got)
	}
}

func TestHealthService_AnyDown_StatusDegraded(t *testing.T) {
	svc := NewHealthService("v1",
		Probe{Name: "mongodb", Check: okProbe},
		Probe{Name: "redis", Check: failProbe},
	)
	resp := svc.Check(context.Background())

	if resp.Status != "degraded" {
		t.Errorf("status = %q, want degraded", resp.Status)
	}
	if got := resp.Dependencies["redis"].Status; got != "down" {
		t.Errorf("redis status = %q, want down", got)
	}
	if resp.Dependencies["redis"].Error == "" {
		t.Errorf("redis error should be populated when down")
	}
	if resp.Dependencies["redis"].LatencyMS != 0 {
		t.Errorf("down dependency latency should be 0, got %d", resp.Dependencies["redis"].LatencyMS)
	}
	if got := resp.Dependencies["mongodb"].Status; got != "up" {
		t.Errorf("mongodb status = %q, want up", got)
	}
}

func TestHealthService_SkippedDoesNotAffectAggregation(t *testing.T) {
	svc := NewHealthService("v1", Probe{Name: "mongodb", Check: okProbe})
	svc.MarkSkipped("arcadedb")

	resp := svc.Check(context.Background())

	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok (skipped must not degrade)", resp.Status)
	}
	if got := resp.Dependencies["arcadedb"].Status; got != "skipped" {
		t.Errorf("arcadedb status = %q, want skipped", got)
	}
	if _, ok := resp.Dependencies["arcadedb"]; !ok {
		t.Errorf("arcadedb should be present in dependencies as skipped")
	}
}

func TestHealthService_TimeoutAndConcurrency(t *testing.T) {
	svc := NewHealthService("v1",
		Probe{Name: "fast", Check: okProbe},
		Probe{Name: "slow1", Check: slowProbe},
		Probe{Name: "slow2", Check: slowProbe},
	)
	start := time.Now()
	resp := svc.Check(context.Background())
	elapsed := time.Since(start)

	if resp.Status != "degraded" {
		t.Errorf("status = %q, want degraded (slow probes timed out)", resp.Status)
	}
	if got := resp.Dependencies["fast"].Status; got != "up" {
		t.Errorf("fast status = %q, want up (must not be blocked by slow probes)", got)
	}
	if got := resp.Dependencies["slow1"].Status; got != "down" {
		t.Errorf("slow1 status = %q, want down (timeout)", got)
	}
	if got := resp.Dependencies["slow2"].Status; got != "down" {
		t.Errorf("slow2 status = %q, want down (timeout)", got)
	}
	if elapsed > 3*time.Second {
		t.Errorf("Check took %v, want < 3s (probes should run concurrently, not serially)", elapsed)
	}
}

func TestHealthService_NoProbes_StatusOK(t *testing.T) {
	svc := NewHealthService("v1")
	resp := svc.Check(context.Background())

	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok (no probes = nothing down)", resp.Status)
	}
	if len(resp.Dependencies) != 0 {
		t.Errorf("dependencies should be empty, got %d entries", len(resp.Dependencies))
	}
}

func TestSanitizeError_Empty(t *testing.T) {
	if got := sanitizeError(""); got != "" {
		t.Errorf("sanitizeError(\"\") = %q, want empty", got)
	}
}

func TestSanitizeError_Truncates(t *testing.T) {
	long := strings.Repeat("x", 500)
	got := sanitizeError(long)
	if len(got) > 200 {
		t.Errorf("sanitizeError length = %d, want <= 200", len(got))
	}
}

func TestSanitizeError_MasksUserinfo(t *testing.T) {
	got := sanitizeError("dial tcp://user:secret@host:3306: connection refused")
	if strings.Contains(got, "secret") {
		t.Errorf("sanitizeError leaked password: %q", got)
	}
	if !strings.Contains(got, "user:***") {
		t.Errorf("sanitizeError should mask userinfo as user:***: %q", got)
	}
}

func TestSanitizeError_NoUserinfo_Unchanged(t *testing.T) {
	in := "connection refused"
	got := sanitizeError(in)
	if got != in {
		t.Errorf("sanitizeError(%q) = %q, want unchanged", in, got)
	}
}
