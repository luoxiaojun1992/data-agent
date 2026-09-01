package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agiledragon/gomonkey/v2"

	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
)

// TestModelConfigHandler_ListLLM_Search verifies the SPEC-074 dropdown mode:
// when ?q or ?limit is present, ListLLM routes to SearchLLMModels with the
// parsed q/limit instead of the paginated ListLLMModels path.
func TestModelConfigHandler_ListLLM_Search(t *testing.T) {
	provider := &modelcfg.Provider{}
	var gotQ string
	var gotLimit int
	patches := gomonkey.ApplyMethodFunc(provider, "SearchLLMModels",
		func(_ context.Context, q string, limit int) ([]modelcfg.ModelEntry, int, error) {
			gotQ, gotLimit = q, limit
			return []modelcfg.ModelEntry{{ID: "m1", Name: "M1", Type: modelcfg.ModelTypeLLM}}, 1, nil
		})
	t.Cleanup(patches.Reset)

	h := NewModelConfigHandler(nil, provider)
	c, w := newRBACGin("GET", "/models/list?q=gpt&limit=30")
	h.ListLLM(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotQ != "gpt" || gotLimit != 30 {
		t.Errorf("SearchLLMModels args = (%q,%d), want (gpt,30)", gotQ, gotLimit)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	models, ok := resp["models"].([]any)
	if !ok || len(models) != 1 {
		t.Errorf("expected 1 model in body, got %v", resp["models"])
	}
}

// TestModelConfigHandler_ListLLM_LimitOnly verifies that a bare ?limit= param
// (no q) also enters search mode and uses the clamped limit.
func TestModelConfigHandler_ListLLM_LimitOnly(t *testing.T) {
	provider := &modelcfg.Provider{}
	var gotQ string
	var gotLimit int
	patches := gomonkey.ApplyMethodFunc(provider, "SearchLLMModels",
		func(_ context.Context, q string, limit int) ([]modelcfg.ModelEntry, int, error) {
			gotQ, gotLimit = q, limit
			return []modelcfg.ModelEntry{}, 0, nil
		})
	t.Cleanup(patches.Reset)

	h := NewModelConfigHandler(nil, provider)
	c, w := newRBACGin("GET", "/models/list?limit=500")
	h.ListLLM(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotQ != "" {
		t.Errorf("q = %q, want empty", gotQ)
	}
	if gotLimit != 100 {
		t.Errorf("limit = %d, want 100 (clamped)", gotLimit)
	}
}

// TestModelConfigHandler_ListEmbedding_Search verifies the embedding dropdown
// path routes to SearchEmbeddingModels.
func TestModelConfigHandler_ListEmbedding_Search(t *testing.T) {
	provider := &modelcfg.Provider{}
	var gotQ string
	var gotLimit int
	patches := gomonkey.ApplyMethodFunc(provider, "SearchEmbeddingModels",
		func(_ context.Context, q string, limit int) ([]modelcfg.ModelEntry, int, error) {
			gotQ, gotLimit = q, limit
			return []modelcfg.ModelEntry{{ID: "e1", Name: "E1", Type: modelcfg.ModelTypeEmbedding}}, 1, nil
		})
	t.Cleanup(patches.Reset)

	h := NewModelConfigHandler(nil, provider)
	c, w := newRBACGin("GET", "/admin/models/embedding?q=nomic&limit=20")
	h.ListEmbedding(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotQ != "nomic" || gotLimit != 20 {
		t.Errorf("SearchEmbeddingModels args = (%q,%d), want (nomic,20)", gotQ, gotLimit)
	}
}

// ensure httptest import is referenced even if helpers change.
var _ = httptest.NewRecorder
