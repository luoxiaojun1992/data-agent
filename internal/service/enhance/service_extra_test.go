package enhance

import (
	"context"
	"iter"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"google.golang.org/adk/model"
	genai "google.golang.org/genai"

	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
	"github.com/luoxiaojun1992/data-agent/internal/infra/llmcache"
	"github.com/luoxiaojun1992/data-agent/internal/infra/llmstats"
)

// fakeLLM implements model.LLM for enhance testing.
type fakeLLM struct {
	text string
	err  error
}

func (f *fakeLLM) Name() string { return "fake-enhance" }

func (f *fakeLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		if f.err != nil {
			yield(nil, f.err)
			return
		}
		yield(&model.LLMResponse{Content: genai.NewContentFromText(f.text, "model")}, nil)
	}
}

func TestEnhance_CacheHit(t *testing.T) {
	cache := &llmcache.Cache{}
	patches := gomonkey.ApplyMethodReturn(cache, "GetEnhance", "cached-enhanced-prompt", true)
	defer patches.Reset()

	svc := NewService(nil, cache, nil)
	got := svc.Enhance(context.Background(), "原始提示词")
	if got != "cached-enhanced-prompt" {
		t.Errorf("cache hit: got %q, want cached-enhanced-prompt", got)
	}
}

func TestEnhance_ADKSuccess(t *testing.T) {
	provider := &modelcfg.Provider{}
	cache := &llmcache.Cache{}
	recorder := &llmstats.Recorder{}

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(cache, "GetEnhance", "", false) // cache miss
	patches.ApplyMethodReturn(provider, "BuildLLM", &fakeLLM{text: "ADK优化结果"}, nil)
	patches.ApplyMethodReturn(cache, "SetEnhance") // no-op store
	patches.ApplyMethodReturn(recorder, "Record", nil)

	svc := NewService(provider, cache, recorder)
	got := svc.Enhance(context.Background(), "分析营收")
	if got != "ADK优化结果" {
		t.Errorf("ADK success: got %q, want ADK优化结果", got)
	}
}

func TestEnhance_ADKError_FallsBackToHTTP(t *testing.T) {
	provider := &modelcfg.Provider{}
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(provider, "BuildLLM", (*fakeLLM)(nil), errEnh("model unavailable"))

	svc := &Service{modelCfg: provider}
	got := svc.enhanceViaADK(context.Background(), "分析营收")
	// callEnhanceLLM fails without real endpoint → returns original prompt
	if got != "分析营收" {
		t.Errorf("fallback should return original prompt, got %q", got)
	}
}

func TestEnhance_GenerateContentError_FallsBack(t *testing.T) {
	provider := &modelcfg.Provider{}
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(provider, "BuildLLM", &fakeLLM{err: errEnh("generate failed")}, nil)

	svc := &Service{modelCfg: provider}
	got := svc.enhanceViaADK(context.Background(), "test")
	if got != "test" {
		t.Errorf("generate error fallback: got %q, want original", got)
	}
}

func TestEnhance_GenerateContentEmptyParts_FallsBack(t *testing.T) {
	provider := &modelcfg.Provider{}
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	// LLM returns a response with no content parts
	patches.ApplyMethodReturn(provider, "BuildLLM", &emptyPartsLLM{}, nil)

	svc := &Service{modelCfg: provider}
	got := svc.enhanceViaADK(context.Background(), "test")
	if got != "test" {
		t.Errorf("empty parts fallback: got %q, want original", got)
	}
}

func TestEnhance_CacheStore(t *testing.T) {
	provider := &modelcfg.Provider{}
	cache := &llmcache.Cache{}
	recorder := &llmstats.Recorder{}

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(cache, "GetEnhance", "", false)
	patches.ApplyMethodReturn(provider, "BuildLLM", &fakeLLM{text: "优化后"}, nil)

	stored := false
	patches.ApplyMethod(cache, "SetEnhance", func(_ *llmcache.Cache, _ context.Context, _, _, _ string) {
		stored = true
	})
	patches.ApplyMethodReturn(recorder, "Record", nil)

	svc := NewService(provider, cache, recorder)
	got := svc.Enhance(context.Background(), "原始")
	if got != "优化后" {
		t.Errorf("got %q", got)
	}
	if !stored {
		t.Error("SetEnhance should be called on cache miss")
	}
}

func TestEnhance_RecordTokens(t *testing.T) {
	provider := &modelcfg.Provider{}
	recorder := &llmstats.Recorder{}

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(provider, "BuildLLM", &fakeLLM{text: "优化后"}, nil)

	recorded := false
	patches.ApplyMethod(recorder, "Record", func(_ *llmstats.Recorder, _ context.Context, r llmstats.Record) error {
		recorded = true
		if r.CallPoint != "enhance" {
			t.Errorf("CallPoint = %q, want enhance", r.CallPoint)
		}
		return nil
	})

	svc := NewService(provider, nil, recorder)
	got := svc.Enhance(context.Background(), "原始")
	if got != "优化后" {
		t.Errorf("got %q", got)
	}
	if !recorded {
		t.Error("Record should be called")
	}
}

func TestEnhanceViaADK_NilModelCfg(t *testing.T) {
	svc := &Service{}
	got := svc.enhanceViaADK(context.Background(), "原始提示")
	// callEnhanceLLM fails without endpoint → returns original
	if got != "原始提示" {
		t.Errorf("nil modelCfg: got %q, want original", got)
	}
}

// emptyPartsLLM returns a response with nil Content.
type emptyPartsLLM struct{}

func (e *emptyPartsLLM) Name() string { return "empty" }

func (e *emptyPartsLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{Content: nil}, nil)
	}
}

type errEnh string

func (e errEnh) Error() string { return string(e) }

func TestCallEnhanceLLM_HTTPSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"http优化结果"}}]}`))
	}))
	defer srv.Close()

	t.Setenv("LLM_BASE_URL", srv.URL)
	t.Setenv("LLM_API_KEY", "test-key")

	got := callEnhanceLLM(context.Background(), "原始提示")
	if got != "http优化结果" {
		t.Errorf("HTTP success: got %q, want http优化结果", got)
	}
}

func TestCallEnhanceLLM_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	t.Setenv("LLM_BASE_URL", srv.URL)
	got := callEnhanceLLM(context.Background(), "原始提示")
	if got != "原始提示" {
		t.Errorf("empty choices: got %q, want original", got)
	}
}

func TestCallEnhanceLLM_HTTPError(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "http://127.0.0.1:1") // unreachable
	got := callEnhanceLLM(context.Background(), "原始提示")
	if got != "原始提示" {
		t.Errorf("HTTP error: got %q, want original", got)
	}
}

func TestEnvOrDefault_EnvSet(t *testing.T) {
	t.Setenv("TEST_ENH_VAR", "custom-value")
	if got := envOrDefault("TEST_ENH_VAR", "default"); got != "custom-value" {
		t.Errorf("envOrDefault with env set: got %q, want custom-value", got)
	}
}
