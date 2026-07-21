package im

import (
	"context"
	"errors"
	"testing"

	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

func TestNewBindService(t *testing.T) {
	repo := mockrepo.NewIMBindRepository(t)
	svc := NewBindService(repo)
	if svc == nil {
		t.Fatal("NewBindService should not return nil")
	}
}

func TestBindService_Get_Success(t *testing.T) {
	repo := mockrepo.NewIMBindRepository(t)
	want := map[string]interface{}{"user_id": "u1", "feishu_app_id": "app123"}
	repo.On("Get", context.Background(), "u1").Return(want, nil)

	svc := NewBindService(repo)
	got, err := svc.Get(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["feishu_app_id"] != "app123" {
		t.Errorf("Get: got %v, want feishu_app_id=app123", got)
	}
}

func TestBindService_Get_NotFound(t *testing.T) {
	repo := mockrepo.NewIMBindRepository(t)
	repo.On("Get", context.Background(), "u1").Return((map[string]interface{})(nil), nil)

	svc := NewBindService(repo)
	got, err := svc.Get(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("Get: expected nil for not-found, got %v", got)
	}
}

func TestBindService_Get_Error(t *testing.T) {
	repo := mockrepo.NewIMBindRepository(t)
	repo.On("Get", context.Background(), "u1").Return((map[string]interface{})(nil), errors.New("db error"))

	svc := NewBindService(repo)
	_, err := svc.Get(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBindService_Upsert_Success(t *testing.T) {
	repo := mockrepo.NewIMBindRepository(t)
	data := map[string]interface{}{"feishu_app_id": "app123"}
	repo.On("Upsert", context.Background(), "u1", data).Return(nil)

	svc := NewBindService(repo)
	if err := svc.Upsert(context.Background(), "u1", data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBindService_Upsert_Error(t *testing.T) {
	repo := mockrepo.NewIMBindRepository(t)
	repo.On("Upsert", context.Background(), "u1", (map[string]interface{})(nil)).Return(errors.New("db error"))

	svc := NewBindService(repo)
	if err := svc.Upsert(context.Background(), "u1", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestBindService_Delete_Success(t *testing.T) {
	repo := mockrepo.NewIMBindRepository(t)
	repo.On("Delete", context.Background(), "u1").Return(nil)

	svc := NewBindService(repo)
	if err := svc.Delete(context.Background(), "u1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBindService_Delete_Error(t *testing.T) {
	repo := mockrepo.NewIMBindRepository(t)
	repo.On("Delete", context.Background(), "u1").Return(errors.New("db error"))

	svc := NewBindService(repo)
	if err := svc.Delete(context.Background(), "u1"); err == nil {
		t.Fatal("expected error")
	}
}
