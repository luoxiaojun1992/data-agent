package rbac

import (
	"context"
	"testing"

	"github.com/agiledragon/gomonkey/v2"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
)

// These three list methods share the same skip=(page-1)*pageSize offset
// computation as ListRoles; each is verified once so the pagination math is
// covered on every code path.

func TestListPermissions_SkipCalculation(t *testing.T) {
	s, repo := newRbacSvc()
	var gotSkip, gotLimit int64
	patches := gomonkey.ApplyMethodFunc(repo, "ListPermissions",
		func(_ context.Context, skip, limit int64, q, excludeRoleID string) ([]model.RBACPermission, int64, error) {
			gotSkip, gotLimit = skip, limit
			return nil, 0, nil
		})
	t.Cleanup(patches.Reset)

	_, _, err := s.ListPermissions(context.Background(), 2, 50, "", "")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if gotSkip != 50 || gotLimit != 50 {
		t.Errorf("skip/limit = %d/%d, want 50/50", gotSkip, gotLimit)
	}
}

func TestListRolePermissions_SkipCalculation(t *testing.T) {
	s, repo := newRbacSvc()
	var gotRoleID string
	var gotSkip, gotLimit int64
	patches := gomonkey.ApplyMethodFunc(repo, "ListRolePermissions",
		func(_ context.Context, roleID string, skip, limit int64) ([]model.RBACPermission, int64, error) {
			gotRoleID, gotSkip, gotLimit = roleID, skip, limit
			return nil, 0, nil
		})
	t.Cleanup(patches.Reset)

	_, _, err := s.ListRolePermissions(context.Background(), "r1", 4, 25)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if gotRoleID != "r1" || gotSkip != 75 || gotLimit != 25 {
		t.Errorf("args = (%q,%d,%d), want (r1,75,25)", gotRoleID, gotSkip, gotLimit)
	}
}

func TestListUserRoles_SkipCalculation(t *testing.T) {
	s, repo := newRbacSvc()
	var gotUserID string
	var gotSkip, gotLimit int64
	patches := gomonkey.ApplyMethodFunc(repo, "ListUserRoles",
		func(_ context.Context, userID string, skip, limit int64) ([]model.RBACRole, int64, error) {
			gotUserID, gotSkip, gotLimit = userID, skip, limit
			return nil, 0, nil
		})
	t.Cleanup(patches.Reset)

	_, _, err := s.ListUserRoles(context.Background(), "u1", 1, 10)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if gotUserID != "u1" || gotSkip != 0 || gotLimit != 10 {
		t.Errorf("args = (%q,%d,%d), want (u1,0,10)", gotUserID, gotSkip, gotLimit)
	}
}
