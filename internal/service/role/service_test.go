package role

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	repomocks "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

func TestService_List(t *testing.T) {
	repo := repomocks.NewRoleRepository(t)
	repo.On("List", mock.Anything).Return([]model.Role{{ID: "r1", Name: "custom"}}, nil)
	svc := NewService(repo)
	roles, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	// Fixed roles are always prepended.
	if len(roles) <= 1 {
		t.Errorf("expected fixed + custom roles, got %d", len(roles))
	}
}

func TestService_List_Error(t *testing.T) {
	repo := repomocks.NewRoleRepository(t)
	repo.On("List", mock.Anything).Return(([]model.Role)(nil), errStr("db"))
	svc := NewService(repo)
	_, err := svc.List(context.Background())
	if err == nil {
		t.Error("expected error")
	}
}

func TestService_ListPermissions(t *testing.T) {
	repo := repomocks.NewRoleRepository(t)
	svc := NewService(repo)
	perms := svc.ListPermissions()
	if len(perms) == 0 {
		t.Error("expected non-empty permissions")
	}
}

func TestService_Create(t *testing.T) {
	repo := repomocks.NewRoleRepository(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)
	svc := NewService(repo)
	r, err := svc.Create(context.Background(), "editor", "编辑", []string{"read"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if r.Name != "editor" || r.DisplayName != "编辑" || r.Type != "custom" {
		t.Errorf("role = %+v", r)
	}
}

func TestService_Create_EmptyDisplayName(t *testing.T) {
	repo := repomocks.NewRoleRepository(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)
	svc := NewService(repo)
	r, err := svc.Create(context.Background(), "editor", "", nil)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if r.DisplayName != "editor" {
		t.Errorf("displayName should default to name, got %q", r.DisplayName)
	}
	if len(r.Permissions) != 0 {
		t.Errorf("permissions should be empty slice, got %v", r.Permissions)
	}
}

func TestService_Create_Error(t *testing.T) {
	repo := repomocks.NewRoleRepository(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(errStr("db"))
	svc := NewService(repo)
	_, err := svc.Create(context.Background(), "x", "x", []string{})
	if err == nil {
		t.Error("expected error")
	}
}

func TestService_Update(t *testing.T) {
	repo := repomocks.NewRoleRepository(t)
	repo.On("Update", mock.Anything, "r1", []string{"read"}).Return(nil)
	svc := NewService(repo)
	if err := svc.Update(context.Background(), "r1", []string{"read"}); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestService_Delete(t *testing.T) {
	repo := repomocks.NewRoleRepository(t)
	repo.On("Delete", mock.Anything, "r1").Return(nil)
	svc := NewService(repo)
	if err := svc.Delete(context.Background(), "r1"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

type errStr string

func (e errStr) Error() string { return string(e) }
