package handler

import (
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
)

// UnsetDefault / UpdateModel are the two ModelConfigHandler methods without
// handler-level coverage; the provider-level logic is tested in modelcfg.

func TestModelConfig_UnsetDefault(t *testing.T) {
	h := newModelTestHandler(t, []modelcfg.ModelEntry{
		{ID: "m1", Name: "M1", Type: modelcfg.ModelTypeLLM, IsDefaultFor: []string{"chat"}},
	})
	c, w := ginReq("DELETE", "/models/default", map[string]any{"use_cases": []string{"chat"}})
	h.UnsetDefault(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestModelConfig_UnsetDefault_NoProvider(t *testing.T) {
	h := NewModelConfigHandler(nil, nil)
	c, w := ginReq("DELETE", "/models/default", map[string]any{"use_cases": []string{"chat"}})
	h.UnsetDefault(c)
	if w.Code != 503 {
		t.Errorf("expected 503 when no provider, got %d", w.Code)
	}
}

func TestModelConfig_UnsetDefault_EmptyUseCases(t *testing.T) {
	h := newModelTestHandler(t, nil)
	c, w := ginReq("DELETE", "/models/default", map[string]any{"use_cases": []string{}})
	h.UnsetDefault(c)
	if w.Code != 400 {
		t.Errorf("expected 400 for empty use_cases, got %d", w.Code)
	}
}

func TestModelConfig_UpdateModel(t *testing.T) {
	h := newModelTestHandler(t, []modelcfg.ModelEntry{
		{ID: "m1", Name: "M1", Type: modelcfg.ModelTypeLLM},
	})
	c, w := ginReq("PATCH", "/models/m1", modelcfg.ModelEntry{Name: "Updated"})
	c.Params = gin.Params{{Key: "id", Value: "m1"}}
	h.UpdateModel(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestModelConfig_UpdateModel_NoProvider(t *testing.T) {
	h := NewModelConfigHandler(nil, nil)
	c, w := ginReq("PATCH", "/models/m1", modelcfg.ModelEntry{Name: "X"})
	c.Params = gin.Params{{Key: "id", Value: "m1"}}
	h.UpdateModel(c)
	if w.Code != 503 {
		t.Errorf("expected 503 when no provider, got %d", w.Code)
	}
}

func TestModelConfig_UpdateModel_BadJSON(t *testing.T) {
	h := newModelTestHandler(t, []modelcfg.ModelEntry{
		{ID: "m1", Name: "M1", Type: modelcfg.ModelTypeLLM},
	})
	c, w := ginReq("PATCH", "/models/m1", map[string]any{"type": 123})
	c.Params = gin.Params{{Key: "id", Value: "m1"}}
	h.UpdateModel(c)
	if w.Code != 400 {
		t.Errorf("expected 400 on bad JSON, got %d: %s", w.Code, w.Body.String())
	}
}
