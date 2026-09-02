package handler

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	configmocks "github.com/luoxiaojun1992/data-agent/internal/service/config/mocks"
)

// TestModelConfigHandler_Put_ServiceError verifies Put returns 500 when the
// underlying config service Upsert call fails.
func TestModelConfigHandler_Put_ServiceError(t *testing.T) {
	svc := configmocks.NewService(t)
	svc.On("Upsert", mock.Anything, "key1", "val1", "").Return(errStr("db down"))
	h := NewModelConfigHandler(svc, nil)
	c, w := newModelCfgGin("PUT", "/models", `{"key":"key1","value":"val1"}`)
	h.Put(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestModelConfigHandler_Get_ServiceErrorReturningEmptyList ensures the
// success-path branch with an empty (but non-nil) slice still serializes
// correctly. This exercises the Get happy path with no rows.
func TestModelConfigHandler_Get_ServiceErrorReturningEmptyList(t *testing.T) {
	svc := configmocks.NewService(t)
	svc.On("GetAll", mock.Anything).Return([]model.SystemConfig{}, nil)
	h := NewModelConfigHandler(svc, nil)
	c, w := newModelCfgGin("GET", "/models", "")
	h.Get(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty body")
	}
}

