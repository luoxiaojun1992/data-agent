package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"

	domainknowledge "github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	rbacsvc "github.com/luoxiaojun1992/data-agent/internal/service/rbac"
	kbmocks "github.com/luoxiaojun1992/data-agent/internal/service/knowledge/mocks"
)

// TestDashboardHandler_Get_AllServicesError verifies the Get endpoint still
// returns 200 with zeroed fields when the underlying services fail — the
// dashboard intentionally swallows errors so a partial outage does not break
// the UI.
func TestDashboardHandler_Get_AllServicesError(t *testing.T) {
	kb := kbmocks.NewKnowledgeService(t)
	kb.On("ListAllDocs", 1, 1).Return(([]*domainknowledge.KnowledgeDoc)(nil), int64(0), errors.New("kb db down"))

	h := NewDashboardHandler(kb, nil)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", "u1")
	c.Request = httptest.NewRequest("GET", "/dashboard", nil)
	h.Get(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 even when all services fail, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["kb_docs"]; !ok {
		t.Errorf("missing kb_docs field: %+v", resp)
	}
}

// TestRegisterDashboardRoutes verifies that RegisterDashboardRoutes wires the
// /api/v1/dashboard and /api/v1/dashboard/trends routes (SPEC-072), now guarded
// by stats:view RBAC (SPEC-084).
func TestRegisterDashboardRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	kb := kbmocks.NewKnowledgeService(t)
	kb.On("ListAllDocs", 1, 1).Return([]*domainknowledge.KnowledgeDoc{{ID: "d1"}}, int64(1), nil)
	h := NewDashboardHandler(kb, nil)
	midd := func(c *gin.Context) { c.Set("user_id", "u1"); c.Set("role", "user"); c.Next() }

	// Stub HasPermission to grant stats:view so the RBAC guard passes.
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "HasPermission",
		func(_ context.Context, _ string, _ string) (bool, error) {
			return true, nil
		})
	t.Cleanup(patches.Reset)

	RegisterDashboardRoutes(r, midd, h, svc)

	req := httptest.NewRequest("GET", "/api/v1/dashboard", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "kb_docs") {
		t.Errorf("expected kb_docs field in body, got %s", w.Body.String())
	}

	reqTrends := httptest.NewRequest("GET", "/api/v1/dashboard/trends?granularity=day", nil)
	wTrends := httptest.NewRecorder()
	r.ServeHTTP(wTrends, reqTrends)
	if wTrends.Code != http.StatusOK {
		t.Errorf("expected 200 for trends, got %d: %s", wTrends.Code, wTrends.Body.String())
	}
	if !strings.Contains(wTrends.Body.String(), "token_tokens") {
		t.Errorf("expected token_tokens field in trends body, got %s", wTrends.Body.String())
	}
	if !strings.Contains(wTrends.Body.String(), "granularity") {
		t.Errorf("expected granularity field in trends body, got %s", wTrends.Body.String())
	}
}
