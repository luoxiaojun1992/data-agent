package enhance

import (
	"context"
	"testing"
)

// TestEnhance_NilDeps_Fallback verifies that with no model config, no cache,
// and no recorder, the service falls back to callEnhanceLLM (which fails
// without a real endpoint) and returns the original prompt unchanged.
func TestEnhance_NilDeps_Fallback(t *testing.T) {
	svc := NewService(nil, nil, nil)
	got := svc.Enhance(context.Background(), "分析营收")
	if got != "分析营收" {
		t.Errorf("expected original prompt on fallback, got %q", got)
	}
}

// TestEnhance_NilModelCfg_NoCache verifies the nil-modelCfg branch of
// enhanceViaADK returns the callEnhanceLLM fallback.
func TestEnhance_NilModelCfg_NoCache(t *testing.T) {
	svc := &Service{}
	got := svc.enhanceViaADK(context.Background(), "prompt")
	if got != "prompt" {
		t.Errorf("expected fallback to original prompt, got %q", got)
	}
}

// TestEnvOrDefault verifies the env helper.
func TestEnvOrDefault(t *testing.T) {
	if got := envOrDefault("UNSET_VAR_X", "def"); got != "def" {
		t.Errorf("expected default, got %q", got)
	}
}

// TestRecordTokens_NilRecorder verifies recordTokens is a no-op when recorder
// is nil (no panic).
func TestRecordTokens_NilRecorder(t *testing.T) {
	svc := &Service{}
	svc.recordTokens(context.Background(), "p", "e")
}
