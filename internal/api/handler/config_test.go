package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	configmocks "github.com/luoxiaojun1992/data-agent/internal/service/config/mocks"
)

func newCfgGin(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func TestConfigHandler_Get(t *testing.T) {
	cfgSvc := configmocks.NewService(t)
	cfgSvc.On("List", mock.Anything, 1, 20).Return([]model.SystemConfig{{Key: "k"}}, int64(1), nil)
	h := NewConfigHandler(cfgSvc)
	c, w := newCfgGin("GET", "/sysconfig/system", "")
	h.Get(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestConfigHandler_Get_Error(t *testing.T) {
	cfgSvc := configmocks.NewService(t)
	cfgSvc.On("List", mock.Anything, 1, 20).Return(([]model.SystemConfig)(nil), int64(0), errStr("db"))
	h := NewConfigHandler(cfgSvc)
	c, w := newCfgGin("GET", "/sysconfig/system", "")
	h.Get(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestConfigHandler_Put(t *testing.T) {
	cfgSvc := configmocks.NewService(t)
	cfgSvc.On("Upsert", mock.Anything, "k", "v", "").Return(nil)
	h := NewConfigHandler(cfgSvc)
	c, w := newCfgGin("PUT", "/sysconfig/system", `{"key":"k","value":"v"}`)
	h.Put(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestConfigHandler_Put_InvalidBody(t *testing.T) {
	h := NewConfigHandler(nil)
	c, w := newCfgGin("PUT", "/sysconfig/system", "not-json")
	h.Put(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

