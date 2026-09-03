// Package monitor provides system statistics and dependency health probing.
// This file implements the SPEC-079 health-check service: a dependency-agnostic
// aggregator that runs injected probe closures concurrently and reports each
// dependency's up/down/skipped state plus latency.
package monitor

import (
	"context"
	"sync"
	"time"
)

// probeTimeout bounds each individual dependency probe so a single hung
// dependency cannot stall the whole health check.
const probeTimeout = 2 * time.Second

// DependencyStatus is the per-dependency health result.
type DependencyStatus struct {
	Status    string `json:"status"`               // "up" | "down" | "skipped"
	LatencyMS int64  `json:"latency_ms,omitempty"` // only meaningful for "up"
	Error     string `json:"error,omitempty"`      // sanitized reason for "down"
}

// HealthResponse is the full health-check payload returned to the frontend.
// Top-level LatencyMS is the total wall-clock time of one Check (the backend
// API latency), independent of the per-dependency latencies.
type HealthResponse struct {
	Status       string                     `json:"status"` // "ok" | "degraded"
	Time         string                     `json:"time"`
	Version      string                     `json:"version,omitempty"`
	UptimeSec    int64                      `json:"uptime_seconds"`
	LatencyMS    int64                      `json:"latency_ms"`
	Dependencies map[string]DependencyStatus `json:"dependencies"`
}

// Probe is a single dependency health check. Check is a closure supplied by the
// wiring layer (wire.go) so the service stays free of any infra import — it
// only needs to return nil (healthy) or a non-nil error (unhealthy).
type Probe struct {
	Name  string
	Check func(ctx context.Context) error
}

// HealthService aggregates dependency probes into a single health response.
// It owns no infra clients — every dependency is injected as a Probe closure,
// preserving the DDD layering rule (service never imports infra types).
type HealthService struct {
	probes    []Probe
	skipped   []string // conditional deps configured out → reported as "skipped"
	version   string
	startTime time.Time
}

// NewHealthService builds a HealthService from a version string and the list of
// active probes. startTime is shared with the process start (monitor.go) so
// uptime is consistent with /api/v1/system/stats.
func NewHealthService(version string, probes ...Probe) *HealthService {
	return &HealthService{
		probes:    probes,
		version:   version,
		startTime: startTime,
	}
}

// MarkSkipped records conditional dependencies that are configured out (e.g.
// ArcadeDB without ARCADE_URI, Presidio with pii_redaction_enabled=false).
// They appear in the response as "skipped" and never affect the degraded
// aggregation.
func (s *HealthService) MarkSkipped(names ...string) {
	s.skipped = append(s.skipped, names...)
}

// Check probes every registered dependency concurrently (each bounded by
// probeTimeout) and aggregates the results. Any required dependency down yields
// status "degraded"; otherwise "ok". The top-level LatencyMS records the total
// elapsed wall-clock time of the whole check.
func (s *HealthService) Check(ctx context.Context) HealthResponse {
	start := time.Now()
	deps := make(map[string]DependencyStatus, len(s.probes)+len(s.skipped))

	// Conditional deps that are configured out are reported as skipped.
	for _, name := range s.skipped {
		deps[name] = DependencyStatus{Status: "skipped"}
	}

	type result struct {
		name string
		st   DependencyStatus
	}
	results := make(chan result, len(s.probes))
	var wg sync.WaitGroup
	for _, p := range s.probes {
		wg.Add(1)
		go func(p Probe) {
			defer wg.Done()
			probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
			defer cancel()
			t0 := time.Now()
			err := p.Check(probeCtx)
			elapsed := time.Since(t0)
			st := DependencyStatus{Status: "up", LatencyMS: elapsed.Milliseconds()}
			if elapsed > 0 && st.LatencyMS == 0 {
				// 亚毫秒探活（如 docker 内网 redis PING）向上取整到 1ms，
				// 否则 omitempty 会吞掉 latency_ms=0，前端看不到该依赖的延时。
				st.LatencyMS = 1
			}
			if err != nil {
				st.Status = "down"
				st.LatencyMS = 0
				st.Error = sanitizeError(err.Error())
			}
			results <- result{name: p.Name, st: st}
		}(p)
	}
	wg.Wait()
	close(results)

	for r := range results {
		deps[r.name] = r.st
	}

	status := "ok"
	for _, d := range deps {
		if d.Status == "down" {
			status = "degraded"
			break
		}
	}

	return HealthResponse{
		Status:       status,
		Time:         time.Now().UTC().Format(time.RFC3339),
		Version:      s.version,
		UptimeSec:    int64(time.Since(s.startTime).Seconds()),
		LatencyMS:    time.Since(start).Milliseconds(),
		Dependencies: deps,
	}
}

// sanitizeError trims and length-bounds an error message so the health endpoint
// never leaks connection strings, credentials, or unbounded text. Connection
// errors (dial/refused/timeout) are safe, but any userinfo in a URL is masked.
func sanitizeError(msg string) string {
	if msg == "" {
		return ""
	}
	const maxLen = 200
	if len(msg) > maxLen {
		msg = msg[:maxLen]
	}
	// Mask "scheme://user:pass@host" userinfo that may appear in a URL.
	if i := indexAfterScheme(msg); i >= 0 {
		if at := indexByte(msg[i:], '@'); at >= 0 {
			userinfo := msg[i : i+at]
			if colon := indexByte(userinfo, ':'); colon >= 0 {
				msg = msg[:i] + userinfo[:colon] + ":***" + msg[i+at:]
			}
		}
	}
	return msg
}

func indexAfterScheme(s string) int {
	// find "://" occurrence.
	for i := 0; i+2 < len(s); i++ {
		if s[i] == ':' && s[i+1] == '/' && s[i+2] == '/' {
			return i + 3
		}
	}
	return -1
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
