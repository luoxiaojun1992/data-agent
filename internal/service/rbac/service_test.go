package rbac

import (
	"context"
	"testing"

	"github.com/agiledragon/gomonkey/v2"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
)

// newTestService builds a Service whose concrete *mongo.RBACRepository is a
// bare (DB-less) instance; individual tests patch its methods with gomonkey
// and assert the argument passthrough from the service layer to the repo.
func newTestService(t *testing.T) *Service {
	t.Helper()
	return &Service{repo: &mongo.RBACRepository{}}
}

func TestListRoles_Passthrough(t *testing.T) {
	svc := newTestService(t)
	var gotSkip, gotLimit int64
	var gotParentID, gotQ, gotExclude string
	patches := gomonkey.ApplyMethodFunc(svc.repo, "ListRoles",
		func(_ context.Context, skip, limit int64, parentID, q, excludeUserID string) ([]model.RBACRole, int64, error) {
			gotSkip, gotLimit, gotParentID, gotQ, gotExclude = skip, limit, parentID, q, excludeUserID
			return []model.RBACRole{{ID: "r1", DisplayName: "R1"}}, int64(7), nil
		})
	t.Cleanup(patches.Reset)

	roles, total, err := svc.ListRoles(context.Background(), 2, 20, "p1", "adm", "u1")
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	if gotSkip != 20 {
		t.Errorf("skip = %d, want 20 ((page-1)*pageSize)", gotSkip)
	}
	if gotLimit != 20 {
		t.Errorf("limit = %d, want 20", gotLimit)
	}
	if gotParentID != "p1" || gotQ != "adm" || gotExclude != "u1" {
		t.Errorf("args = (%q,%q,%q), want (p1,adm,u1)", gotParentID, gotQ, gotExclude)
	}
	if total != 7 {
		t.Errorf("total = %d, want 7", total)
	}
	if len(roles) != 1 || roles[0].ID != "r1" {
		t.Errorf("roles = %+v, want single r1", roles)
	}
}

func TestAvailableParents_Passthrough(t *testing.T) {
	svc := newTestService(t)
	var gotLevel int
	var gotQ string
	var gotLimit int64
	patches := gomonkey.ApplyMethodFunc(svc.repo, "AvailableParents",
		func(_ context.Context, level int, q string, _ int64, limit int64) ([]model.RBACRole, error) {
			gotLevel, gotQ, gotLimit = level, q, limit
			return []model.RBACRole{{ID: "p1", Level: 0}}, nil
		})
	t.Cleanup(patches.Reset)

	parents, err := svc.AvailableParents(context.Background(), 2, "sys", 30)
	if err != nil {
		t.Fatalf("AvailableParents: %v", err)
	}
	if gotLevel != 2 {
		t.Errorf("level = %d, want 2", gotLevel)
	}
	if gotQ != "sys" {
		t.Errorf("q = %q, want sys", gotQ)
	}
	if gotLimit != 30 {
		t.Errorf("limit = %d, want 30", gotLimit)
	}
	if len(parents) != 1 || parents[0].ID != "p1" {
		t.Errorf("parents = %+v, want single p1", parents)
	}
}

func TestListParentCandidates_Passthrough(t *testing.T) {
	svc := newTestService(t)
	var gotQ string
	var gotLimit int64
	patches := gomonkey.ApplyMethodFunc(svc.repo, "ListParentCandidates",
		func(_ context.Context, q string, _ int64, limit int64) ([]model.RBACRole, error) {
			gotQ, gotLimit = q, limit
			return []model.RBACRole{{ID: "c1", Level: 1}}, nil
		})
	t.Cleanup(patches.Reset)

	candidates, err := svc.ListParentCandidates(context.Background(), "anal", 15)
	if err != nil {
		t.Fatalf("ListParentCandidates: %v", err)
	}
	if gotQ != "anal" {
		t.Errorf("q = %q, want anal", gotQ)
	}
	if gotLimit != 15 {
		t.Errorf("limit = %d, want 15", gotLimit)
	}
	if len(candidates) != 1 || candidates[0].ID != "c1" {
		t.Errorf("candidates = %+v, want single c1", candidates)
	}
}

func TestListPermissions_Passthrough(t *testing.T) {
	svc := newTestService(t)
	var gotSkip, gotLimit int64
	var gotQ, gotExclude string
	patches := gomonkey.ApplyMethodFunc(svc.repo, "ListPermissions",
		func(_ context.Context, skip, limit int64, q, excludeRoleID string) ([]model.RBACPermission, int64, error) {
			gotSkip, gotLimit, gotQ, gotExclude = skip, limit, q, excludeRoleID
			return []model.RBACPermission{{ID: "perm1", Key: "kb:view"}}, int64(3), nil
		})
	t.Cleanup(patches.Reset)

	perms, total, err := svc.ListPermissions(context.Background(), 1, 50, "kb", "role1")
	if err != nil {
		t.Fatalf("ListPermissions: %v", err)
	}
	if gotSkip != 0 {
		t.Errorf("skip = %d, want 0", gotSkip)
	}
	if gotLimit != 50 {
		t.Errorf("limit = %d, want 50", gotLimit)
	}
	if gotQ != "kb" || gotExclude != "role1" {
		t.Errorf("args = (%q,%q), want (kb,role1)", gotQ, gotExclude)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(perms) != 1 || perms[0].Key != "kb:view" {
		t.Errorf("perms = %+v, want single kb:view", perms)
	}
}
