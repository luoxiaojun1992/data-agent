// Package pii implements the PII redaction client that fronts Microsoft
// Presidio's analyzer + anonymizer REST services (SPEC-068). PIIRedactor is the
// concrete implementation of security.Redactor, shared by KB upload and the
// model input/output auditor.
package pii

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
)

// ErrDisabled is returned by Redact when the pii_redaction_enabled switch is
// off. Callers can distinguish "switch off" (skip redaction) from "service
// failure" (fail-closed) via errors.Is.
var ErrDisabled = errors.New("pii redaction disabled by config")

// Config configures the PII redactor.
type Config struct {
	AnalyzerURL   string
	AnonymizerURL string
	// Enabled reports whether PII redaction is on (reads the system config
	// switch `pii_redaction_enabled`). nil = always enabled.
	Enabled func() bool
	// HTTPClient overrides the default client (tests).
	HTTPClient *http.Client
}

// PIIRedactor is the security.Redactor implementation that calls the
// presidio-analyzer and presidio-anonymizer services.
type PIIRedactor struct {
	analyzerURL   string
	anonymizerURL string
	enabled       func() bool
	client        *http.Client
}

// New creates a PIIRedactor. If either URL is empty the redactor is inert
// (Redact returns an error so callers fall back to regex rules).
func New(cfg Config) *PIIRedactor {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &PIIRedactor{
		analyzerURL:   cfg.AnalyzerURL,
		anonymizerURL: cfg.AnonymizerURL,
		enabled:       cfg.Enabled,
		client:        client,
	}
}

// Redact runs analyze → anonymize against the presidio services and returns
// the redacted text. It returns an error when the switch is off, the services
// are unconfigured, or a call fails — the caller decides whether to fall back
// to regex rules or abort.
func (r *PIIRedactor) Redact(ctx context.Context, text string) (string, error) {
	if text == "" {
		return text, nil
	}
	if r.enabled != nil && !r.enabled() {
		return "", ErrDisabled
	}
	if r.analyzerURL == "" || r.anonymizerURL == "" {
		return "", fmt.Errorf("presidio services not configured")
	}

	results, err := r.analyze(ctx, text)
	if err != nil {
		return "", fmt.Errorf("presidio analyze: %w", err)
	}
	if len(results) == 0 {
		return text, nil // no PII detected
	}
	return r.anonymize(ctx, text, results)
}

type analyzeResult struct {
	EntityType string  `json:"entity_type"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Score      float64 `json:"score"`
}

func (r *PIIRedactor) analyze(ctx context.Context, text string) ([]analyzeResult, error) {
	body, err := json.Marshal(map[string]any{"text": text, "language": "en"})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.analyzerURL+"/analyze", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("analyze status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var results []analyzeResult
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, fmt.Errorf("decode analyze response: %w", err)
	}
	return results, nil
}

type anonymizeResponse struct {
	Text  string `json:"text"`
	Items []any  `json:"items"`
}

func (r *PIIRedactor) anonymize(ctx context.Context, text string, results []analyzeResult) (string, error) {
	body, err := json.Marshal(map[string]any{
		"text":             text,
		"analyzer_results": results,
		"anonymizers": map[string]any{
			"DEFAULT": map[string]any{"type": "replace", "new_value": "<PII>"},
		},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.anonymizerURL+"/anonymize", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anonymize status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var out anonymizeResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("decode anonymize response: %w", err)
	}
	return out.Text, nil
}

// Health probes both Presidio services' /health endpoints (SPEC-079, aligned
// with the docker-compose healthchecks). Returns an error when either service
// is unconfigured or unhealthy. When the redaction switch is off, callers
// should mark this dependency "skipped" instead of probing.
func (r *PIIRedactor) Health(ctx context.Context) error {
	if r == nil || r.analyzerURL == "" || r.anonymizerURL == "" {
		return fmt.Errorf("presidio services not configured")
	}
	if err := healthEndpoint(ctx, r.client, r.analyzerURL); err != nil {
		return fmt.Errorf("presidio analyzer: %w", err)
	}
	if err := healthEndpoint(ctx, r.client, r.anonymizerURL); err != nil {
		return fmt.Errorf("presidio anonymizer: %w", err)
	}
	return nil
}

func healthEndpoint(ctx context.Context, client *http.Client, baseURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health status %d", resp.StatusCode)
	}
	return nil
}

// Compile-time assertion: PIIRedactor implements security.Redactor.
var _ security.Redactor = (*PIIRedactor)(nil)
