package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
)

func newModelTestHandler(t *testing.T, entries []modelcfg.ModelEntry) *ModelConfigHandler {
	t.Helper()
	repo := mockrepo.NewSysConfigRepository(t)
	raw, _ := json.Marshal(entries)
	cfg := &model.SystemConfig{Namespace: "model", Key: "models", Value: string(raw)}
	repo.On("Get", mock.Anything, "model", "models").Return(cfg, nil)
	repo.On("GetAll", mock.Anything, "model").Maybe().Return([]model.SystemConfig{*cfg}, nil)
	repo.On("Upsert", mock.Anything, "model", "models", mock.Anything).Maybe().Return(nil)
	p := modelcfg.NewProvider(repo)
	return NewModelConfigHandler(nil, p)
}

func ginReq(method, path string, body any) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	c.Request = httptest.NewRequest(method, path, &buf)
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func TestModelConfig_ListLLM(t *testing.T) {
	h := newModelTestHandler(t, []modelcfg.ModelEntry{
		{ID: "m1", Name: "M1", Type: modelcfg.ModelTypeLLM},
		{ID: "m2", Name: "M2", Type: modelcfg.ModelTypeLLM},
		{ID: "e1", Name: "E1", Type: modelcfg.ModelTypeEmbedding},
	})
	c, w := ginReq("GET", "/models/list?page=1&page_size=10", nil)
	h.ListLLM(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Models []modelcfg.ModelEntry `json:"models"`
		Total  int                   `json:"total"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2 (LLM only)", resp.Total)
	}
	if len(resp.Models) != 2 {
		t.Errorf("models = %d, want 2", len(resp.Models))
	}
}

func TestModelConfig_AddModel(t *testing.T) {
	h := newModelTestHandler(t, nil)
	c, w := ginReq("POST", "/models", modelcfg.ModelEntry{Name: "NewModel", Type: modelcfg.ModelTypeLLM})
	h.AddModel(c)
	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var saved modelcfg.ModelEntry
	_ = json.Unmarshal(w.Body.Bytes(), &saved)
	if saved.ID == "" {
		t.Error("auto-generated ID should not be empty")
	}
	if saved.Name != "NewModel" {
		t.Errorf("name = %s, want NewModel", saved.Name)
	}
}

func TestModelConfig_DeleteModel(t *testing.T) {
	h := newModelTestHandler(t, []modelcfg.ModelEntry{
		{ID: "m1", Name: "M1", Type: modelcfg.ModelTypeLLM},
	})
	c, w := ginReq("DELETE", "/models/m1", nil)
	c.Params = gin.Params{{Key: "id", Value: "m1"}}
	h.DeleteModel(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestModelConfig_DeleteModel_Idempotent(t *testing.T) {
	h := newModelTestHandler(t, nil)
	c, w := ginReq("DELETE", "/models/nonexistent", nil)
	c.Params = gin.Params{{Key: "id", Value: "nonexistent"}}
	h.DeleteModel(c)
	if w.Code != 200 {
		t.Errorf("idempotent delete should return 200, got %d", w.Code)
	}
}

func TestModelConfig_SetDefault(t *testing.T) {
	h := newModelTestHandler(t, []modelcfg.ModelEntry{
		{ID: "m1", Name: "M1", Type: modelcfg.ModelTypeLLM, IsDefault: true},
		{ID: "m2", Name: "M2", Type: modelcfg.ModelTypeLLM},
	})
	c, w := ginReq("PATCH", "/models/m2/default", nil)
	c.Params = gin.Params{{Key: "id", Value: "m2"}}
	h.SetDefault(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestModelConfig_SetDefault_NotFound(t *testing.T) {
	h := newModelTestHandler(t, []modelcfg.ModelEntry{
		{ID: "m1", Name: "M1", Type: modelcfg.ModelTypeLLM},
	})
	c, w := ginReq("PATCH", "/models/nonexistent/default", nil)
	c.Params = gin.Params{{Key: "id", Value: "nonexistent"}}
	h.SetDefault(c)
	if w.Code != 400 {
		t.Errorf("expected 400 for nonexistent, got %d", w.Code)
	}
}

func TestModelConfig_Get_Paginated(t *testing.T) {
	h := newModelTestHandler(t, []modelcfg.ModelEntry{
		{ID: "m1", Name: "M1", Type: modelcfg.ModelTypeLLM},
	})
	c, w := ginReq("GET", "/models?page=1&page_size=10&type=llm", nil)
	c.Request = httptest.NewRequest("GET", "/models?page=1&page_size=10&type=llm", nil)
	h.Get(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Models []modelcfg.ModelEntry `json:"models"`
		Total  int                   `json:"total"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Total)
	}
}

func TestModelConfig_ListLLM_NoProvider(t *testing.T) {
	h := NewModelConfigHandler(nil, nil)
	c, w := ginReq("GET", "/models/list", nil)
	h.ListLLM(c)
	if w.Code != 503 {
		t.Errorf("expected 503 when no provider, got %d", w.Code)
	}
}

func TestModelConfig_AddModel_NoProvider(t *testing.T) {
	h := NewModelConfigHandler(nil, nil)
	c, w := ginReq("POST", "/models", modelcfg.ModelEntry{Name: "X"})
	h.AddModel(c)
	if w.Code != 503 {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestModelConfig_DeleteModel_NoProvider(t *testing.T) {
	h := NewModelConfigHandler(nil, nil)
	c, w := ginReq("DELETE", "/models/x", nil)
	c.Params = gin.Params{{Key: "id", Value: "x"}}
	h.DeleteModel(c)
	if w.Code != 503 {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestModelConfig_SetDefault_NoProvider(t *testing.T) {
	h := NewModelConfigHandler(nil, nil)
	c, w := ginReq("PATCH", "/models/x/default", nil)
	c.Params = gin.Params{{Key: "id", Value: "x"}}
	h.SetDefault(c)
	if w.Code != 503 {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

// ensure context import used
var _ = context.Background
