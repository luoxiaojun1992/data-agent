package user

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	repomocks "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

type fakeHasher struct {
	err error
}

func (f *fakeHasher) Hash(password string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return "hashed:" + password, nil
}

func TestService_List(t *testing.T) {
	repo := repomocks.NewUserRepository(t)
	repo.On("ListSorted", mock.Anything, "admin", int64(0), int64(50), "", "").Return([]model.User{{ID: "u1"}}, int64(1), nil)
	svc := NewService(repo, &fakeHasher{})
	users, total, err := svc.List(context.Background(), "admin", 0, 50, "", "")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(users) != 1 || total != 1 {
		t.Errorf("users=%v total=%d", users, total)
	}
}

func TestService_Get(t *testing.T) {
	repo := repomocks.NewUserRepository(t)
	repo.On("FindByID", mock.Anything, "u1").Return(&model.User{ID: "u1", Username: "alice"}, nil)
	svc := NewService(repo, &fakeHasher{})
	u, err := svc.Get(context.Background(), "u1")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if u.Username != "alice" {
		t.Errorf("username = %v", u.Username)
	}
}

func TestService_Create(t *testing.T) {
	repo := repomocks.NewUserRepository(t)
	repo.On("FindByUsername", mock.Anything, "bob").Return((*model.User)(nil), nil)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)
	svc := NewService(repo, &fakeHasher{})
	u, err := svc.Create(context.Background(), "bob", "secret", "admin")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if u.Username != "bob" || u.PasswordHash != "hashed:secret" || u.Role != model.RoleAdmin {
		t.Errorf("user = %+v", u)
	}
}

func TestService_Create_Duplicate(t *testing.T) {
	repo := repomocks.NewUserRepository(t)
	repo.On("FindByUsername", mock.Anything, "bob").Return(&model.User{ID: "u1"}, nil)
	svc := NewService(repo, &fakeHasher{})
	_, err := svc.Create(context.Background(), "bob", "secret", "admin")
	if err != ErrDuplicate {
		t.Errorf("expected ErrDuplicate, got %v", err)
	}
}

func TestService_Create_HashError(t *testing.T) {
	repo := repomocks.NewUserRepository(t)
	repo.On("FindByUsername", mock.Anything, "bob").Return((*model.User)(nil), nil)
	svc := NewService(repo, &fakeHasher{err: errStr("hash fail")})
	_, err := svc.Create(context.Background(), "bob", "secret", "admin")
	if err == nil {
		t.Error("expected hash error")
	}
}

func TestService_UpdateRole(t *testing.T) {
	repo := repomocks.NewUserRepository(t)
	repo.On("UpdateRole", mock.Anything, "u1", model.RoleAdmin).Return(nil)
	svc := NewService(repo, &fakeHasher{})
	if err := svc.UpdateRole(context.Background(), "u1", model.RoleAdmin); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestService_ToggleStatus_Disable(t *testing.T) {
	repo := repomocks.NewUserRepository(t)
	repo.On("FindByID", mock.Anything, "u1").Return(&model.User{ID: "u1", Status: model.StatusEnabled}, nil)
	repo.On("UpdateStatus", mock.Anything, "u1", model.StatusDisabled).Return(nil)
	svc := NewService(repo, &fakeHasher{})
	if err := svc.ToggleStatus(context.Background(), "u1"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestService_ToggleStatus_Enable(t *testing.T) {
	repo := repomocks.NewUserRepository(t)
	repo.On("FindByID", mock.Anything, "u1").Return(&model.User{ID: "u1", Status: model.StatusDisabled}, nil)
	repo.On("UpdateStatus", mock.Anything, "u1", model.StatusEnabled).Return(nil)
	svc := NewService(repo, &fakeHasher{})
	if err := svc.ToggleStatus(context.Background(), "u1"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestService_ToggleStatus_NotFound(t *testing.T) {
	repo := repomocks.NewUserRepository(t)
	repo.On("FindByID", mock.Anything, "u1").Return((*model.User)(nil), errStr("not found"))
	svc := NewService(repo, &fakeHasher{})
	if err := svc.ToggleStatus(context.Background(), "u1"); err == nil {
		t.Error("expected error for missing user")
	}
}

func TestService_Delete(t *testing.T) {
	repo := repomocks.NewUserRepository(t)
	repo.On("Delete", mock.Anything, "u1").Return(nil)
	svc := NewService(repo, &fakeHasher{})
	if err := svc.Delete(context.Background(), "u1"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestNewBcryptHasher(t *testing.T) {
	h := NewBcryptHasher()
	if h == nil {
		t.Error("hasher should not be nil")
	}
	hash, err := h.Hash("test")
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	if hash == "test" {
		t.Error("hash should not equal plaintext")
	}
}

type errStr string

func (e errStr) Error() string { return string(e) }
