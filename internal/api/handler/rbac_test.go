package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	rbacsvc "github.com/luoxiaojun1992/data-agent/internal/service/rbac"
)

func newRBACGin(method, path string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, nil)
	return c, w
}

// TestDropdownLimit verifies the ?limit= query parser: default 20, clamps to
// [1,100], ignores invalid input (SPEC-074 §4.1).
func TestDropdownLimit(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"", 20}, {"0", 20}, {"-3", 20}, {"abc", 20},
		{"1", 1}, {"30", 30}, {"100", 100}, {"500", 100},
	}
	for _, tc := range cases {
		c, _ := newRBACGin("GET", "/x")
		if tc.raw != "" {
			c.Request.URL.RawQuery = "limit=" + tc.raw
		}
		if got := dropdownLimit(c); got != tc.want {
			t.Errorf("dropdownLimit(%q) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}

func TestRBACHandler_ListRoles_DropdownSearch(t *testing.T) {
	svc := &rbacsvc.Service{}
	var gotPage, gotPageSize int
	var gotParentID, gotQ, gotExclude string
	patches := gomonkey.ApplyMethodFunc(svc, "ListRoles",
		func(_ context.Context, page, pageSize int, parentID, q, excludeUserID string) ([]model.RBACRole, int64, error) {
			gotPage, gotPageSize, gotParentID, gotQ, gotExclude = page, pageSize, parentID, q, excludeUserID
			return []model.RBACRole{{ID: "r1", DisplayName: "Admin"}}, int64(1), nil
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/admin/rbac/roles?q=adm&limit=30&exclude_user_id=u1")
	h.ListRoles(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotPage != 1 || gotPageSize != 30 {
		t.Errorf("page/pageSize = %d/%d, want 1/30", gotPage, gotPageSize)
	}
	if gotParentID != "" || gotQ != "adm" || gotExclude != "u1" {
		t.Errorf("args = (%q,%q,%q), want (\"\",adm,u1)", gotParentID, gotQ, gotExclude)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["total"].(float64) != 1 {
		t.Errorf("resp total = %v, want 1", resp["total"])
	}
}

func TestRBACHandler_ListRoles_LimitClamped(t *testing.T) {
	svc := &rbacsvc.Service{}
	var gotPageSize int
	patches := gomonkey.ApplyMethodFunc(svc, "ListRoles",
		func(_ context.Context, _ int, pageSize int, _, _, _ string) ([]model.RBACRole, int64, error) {
			gotPageSize = pageSize
			return nil, int64(0), nil
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/admin/rbac/roles?limit=500")
	h.ListRoles(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotPageSize != 100 {
		t.Errorf("pageSize = %d, want 100 (clamped)", gotPageSize)
	}
}

func TestRBACHandler_ListPermissions_DropdownSearch(t *testing.T) {
	svc := &rbacsvc.Service{}
	var gotPage, gotPageSize int
	var gotQ, gotExclude string
	patches := gomonkey.ApplyMethodFunc(svc, "ListPermissions",
		func(_ context.Context, page, pageSize int, q, excludeRoleID string) ([]model.RBACPermission, int64, error) {
			gotPage, gotPageSize, gotQ, gotExclude = page, pageSize, q, excludeRoleID
			return []model.RBACPermission{{ID: "p1", Key: "kb:view"}}, int64(2), nil
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/admin/rbac/permissions?q=kb&limit=25&exclude_role_id=role1")
	h.ListPermissions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotPage != 1 || gotPageSize != 25 {
		t.Errorf("page/pageSize = %d/%d, want 1/25", gotPage, gotPageSize)
	}
	if gotQ != "kb" || gotExclude != "role1" {
		t.Errorf("args = (%q,%q), want (kb,role1)", gotQ, gotExclude)
	}
}

func TestRBACHandler_ListParentCandidates(t *testing.T) {
	svc := &rbacsvc.Service{}
	var gotQ string
	var gotLimit int
	patches := gomonkey.ApplyMethodFunc(svc, "ListParentCandidates",
		func(_ context.Context, q string, limit int) ([]model.RBACRole, error) {
			gotQ, gotLimit = q, limit
			return []model.RBACRole{{ID: "p1", Level: 0}}, nil
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/admin/rbac/parent-candidates?q=anal&limit=15")
	h.ListParentCandidates(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotQ != "anal" || gotLimit != 15 {
		t.Errorf("args = (%q,%d), want (anal,15)", gotQ, gotLimit)
	}
	if !strings.Contains(w.Body.String(), "p1") {
		t.Errorf("expected p1 in body, got %s", w.Body.String())
	}
}

func TestRBACHandler_AvailableParents(t *testing.T) {
	svc := &rbacsvc.Service{}
	var gotLevel int
	var gotQ string
	var gotLimit int
	patches := gomonkey.ApplyMethodFunc(svc, "GetRole",
		func(_ context.Context, _ string) (*model.RBACRole, error) {
			return &model.RBACRole{ID: "r2", Level: 2}, nil
		})
	patches.ApplyMethodFunc(svc, "AvailableParents",
		func(_ context.Context, level int, q string, limit int) ([]model.RBACRole, error) {
			gotLevel, gotQ, gotLimit = level, q, limit
			return []model.RBACRole{{ID: "p0", Level: 1}}, nil
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/admin/rbac/roles/r2/available-parents?q=sys&limit=10")
	c.Params = gin.Params{{Key: "id", Value: "r2"}}
	h.AvailableParents(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotLevel != 2 {
		t.Errorf("level = %d, want 2", gotLevel)
	}
	if gotQ != "sys" || gotLimit != 10 {
		t.Errorf("args = (%q,%d), want (sys,10)", gotQ, gotLimit)
	}
}
