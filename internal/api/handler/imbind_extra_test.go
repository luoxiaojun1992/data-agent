package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/service/im"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

func TestIMBindHandler_Get_Success(t *testing.T) {
	repo := mockrepo.NewIMBindRepository(t)
	repo.On("Get", mock.Anything, "u1").Return(map[string]interface{}{"open_id": "ou_123"}, nil)
	svc := im.NewBindService(repo)
	h := NewIMBindHandler(svc)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", "u1")
	c.Request = httptest.NewRequest("GET", "/im/bind", nil)
	h.Get(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "ou_123") {
		t.Errorf("missing open_id in response: %s", body)
	}
	if !strings.Contains(body, "binds") {
		t.Errorf("missing binds wrapper: %s", body)
	}
}

func TestIMBindHandler_Get_EmptyBind(t *testing.T) {
	repo := mockrepo.NewIMBindRepository(t)
	repo.On("Get", mock.Anything, "u1").Return((map[string]interface{})(nil), nil)
	svc := im.NewBindService(repo)
	h := NewIMBindHandler(svc)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", "u1")
	c.Request = httptest.NewRequest("GET", "/im/bind", nil)
	h.Get(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"binds":[]`) {
		t.Errorf("expected empty binds array: %s", w.Body.String())
	}
}

func TestIMBindHandler_Get_RepoError(t *testing.T) {
	repo := mockrepo.NewIMBindRepository(t)
	repo.On("Get", mock.Anything, "u1").Return((map[string]interface{})(nil), errStr("db down"))
	svc := im.NewBindService(repo)
	h := NewIMBindHandler(svc)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", "u1")
	c.Request = httptest.NewRequest("GET", "/im/bind", nil)
	h.Get(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestIMBindHandler_Update_Success(t *testing.T) {
	repo := mockrepo.NewIMBindRepository(t)
	repo.On("Upsert", mock.Anything, "u1", mock.Anything).Return(nil)
	svc := im.NewBindService(repo)
	h := NewIMBindHandler(svc)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", "u1")
	c.Request = httptest.NewRequest("PUT", "/im/bind", strings.NewReader(`{"open_id":"ou_new"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Update(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "ok") {
		t.Errorf("expected status ok: %s", w.Body.String())
	}
}

func TestIMBindHandler_Update_InvalidBody(t *testing.T) {
	repo := mockrepo.NewIMBindRepository(t)
	svc := im.NewBindService(repo)
	h := NewIMBindHandler(svc)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", "u1")
	c.Request = httptest.NewRequest("PUT", "/im/bind", strings.NewReader("not-json"))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Update(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestIMBindHandler_Update_RepoError(t *testing.T) {
	repo := mockrepo.NewIMBindRepository(t)
	repo.On("Upsert", mock.Anything, "u1", mock.Anything).Return(errStr("db down"))
	svc := im.NewBindService(repo)
	h := NewIMBindHandler(svc)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", "u1")
	c.Request = httptest.NewRequest("PUT", "/im/bind", strings.NewReader(`{"open_id":"ou_x"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Update(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ensure context import used.
var _ = context.Background
