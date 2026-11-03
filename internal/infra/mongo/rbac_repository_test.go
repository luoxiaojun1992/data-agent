package mongo

import (
	"context"
	"errors"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	driver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
)

// TestRBACRepository_CreateRole_WithParent covers CreateRole's parent-child
// child_count increment (rbac_repository.go:82, the errcheck-flagged UpdateOne).
func TestRBACRepository_CreateRole_WithParent(t *testing.T) {
	r := &RBACRepository{db: &driver.Database{}}
	var updateCalled bool
	mockColl := &driver.Collection{}

	patches := gomonkey.NewPatches()
	patches.ApplyMethodFunc(r, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) {
			return &model.RBACRole{ID: id, Level: 1}, nil
		})
	patches.ApplyMethod(r.db, "Collection",
		func(_ *driver.Database, name string, opts ...*options.CollectionOptions) *driver.Collection {
			return mockColl
		})
	patches.ApplyMethod(mockColl, "UpdateOne",
		func(_ *driver.Collection, ctx context.Context, filter, update interface{}, opts ...*options.UpdateOptions) (*driver.UpdateResult, error) {
			updateCalled = true
			return &driver.UpdateResult{}, nil
		})
	patches.ApplyMethod(mockColl, "InsertOne",
		func(_ *driver.Collection, ctx context.Context, doc interface{}, opts ...*options.InsertOneOptions) (*driver.InsertOneResult, error) {
			return &driver.InsertOneResult{}, nil
		})
	t.Cleanup(patches.Reset)

	role := &model.RBACRole{ParentID: "parent1"}
	if err := r.CreateRole(context.Background(), role); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if !updateCalled {
		t.Errorf("parent child_count UpdateOne 未执行（应覆盖 rbac_repository.go:82）")
	}
	if role.Level != 2 {
		t.Errorf("role.Level = %d, want 2 (parent.Level+1)", role.Level)
	}
}

// TestRBACRepository_ChangeRoleParent_GetRoleError covers ChangeRoleParent's
// GetRole failure branch. The errcheck-flagged UpdateOne at
// rbac_repository.go:145 lives inside a mongo.Session.WithTransaction callback;
// driver.Session is an interface with an unexported method (session()) and its
// concrete impl (sessionImpl) is unexported, so it cannot be mocked via
// gomonkey. That line is exercised by the seed/UI path instead.
func TestRBACRepository_ChangeRoleParent_GetRoleError(t *testing.T) {
	r := &RBACRepository{db: &driver.Database{}}

	patches := gomonkey.NewPatches()
	patches.ApplyMethodFunc(r, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) {
			return nil, errors.New("role not found")
		})
	t.Cleanup(patches.Reset)

	err := r.ChangeRoleParent(context.Background(), "role1", "new-parent")
	if err == nil {
		t.Fatalf("ChangeRoleParent should propagate GetRole error")
	}
}
