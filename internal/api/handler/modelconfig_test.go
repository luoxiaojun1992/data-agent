package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	configmocks "github.com/luoxiaojun1992/data-agent/internal/service/config/mocks"
)

func newModelCfgGin(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func TestModelConfigHandler_Get(t *testing.T) {
	svc := configmocks.NewService(t)
	svc.On("GetAll", mock.Anything, "models").Return([]model.SystemConfig{{Key: "k", Value: "v"}}, nil)
	h := NewModelConfigHandler(svc, nil)
	c, w := newModelCfgGin("GET", "/models", "")
	h.Get(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	models, _ := resp["models"].([]any)
	if len(models) != 1 {
		t.Errorf("models = %v", models)
	}
}

func TestModelConfigHandler_Get_Error(t *testing.T) {
	svc := configmocks.NewService(t)
	svc.On("GetAll", mock.Anything, "models").Return(([]model.SystemConfig)(nil), errStr("db"))
	h := NewModelConfigHandler(svc, nil)
	c, w := newModelCfgGin("GET", "/models", "")
	h.Get(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestModelConfigHandler_Put(t *testing.T) {
	svc := configmocks.NewService(t)
	svc.On("Upsert", mock.Anything, "models", "key1", "val1").Return(nil)
	h := NewModelConfigHandler(svc, nil)
	c, w := newModelCfgGin("PUT", "/models", `{"key":"key1","value":"val1"}`)
	h.Put(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestModelConfigHandler_Put_InvalidBody(t *testing.T) {
	svc := configmocks.NewService(t)
	h := NewModelConfigHandler(svc, nil)
	c, w := newModelCfgGin("PUT", "/models", "not-json")
	h.Put(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestNewModelConfigHandler(t *testing.T) {
	h := NewModelConfigHandler(nil, nil)
	if h == nil {
		t.Error("handler should not be nil")
	}
}
