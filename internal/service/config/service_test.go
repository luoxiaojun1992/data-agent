package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	repomocks "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

func TestService_GetAll(t *testing.T) {
	repo := repomocks.NewSysConfigRepository(t)
	repo.On("GetAll", mock.Anything).Return([]model.SystemConfig{{Key: "k", Value: "v"}}, nil)
	svc := NewService(repo)
	cfgs, err := svc.GetAll(context.Background())
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(cfgs) != 1 {
		t.Errorf("cfgs = %v", cfgs)
	}
}

func TestService_GetAll_NilReturnsEmpty(t *testing.T) {
	repo := repomocks.NewSysConfigRepository(t)
	repo.On("GetAll", mock.Anything).Return(([]model.SystemConfig)(nil), nil)
	svc := NewService(repo)
	cfgs, err := svc.GetAll(context.Background())
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(cfgs) != 0 {
		t.Errorf("expected empty slice, got %v", cfgs)
	}
}

func TestService_GetAll_Error(t *testing.T) {
	repo := repomocks.NewSysConfigRepository(t)
	repo.On("GetAll", mock.Anything).Return(([]model.SystemConfig)(nil), errStr("db"))
	svc := NewService(repo)
	_, err := svc.GetAll(context.Background())
	if err == nil {
		t.Error("expected error")
	}
}

func TestService_Upsert(t *testing.T) {
	repo := repomocks.NewSysConfigRepository(t)
	repo.On("Upsert", mock.Anything, "k", "v").Return(nil)
	svc := NewService(repo)
	if err := svc.Upsert(context.Background(), "k", "v"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestService_Delete(t *testing.T) {
	repo := repomocks.NewSysConfigRepository(t)
	repo.On("Delete", mock.Anything, "k").Return(nil)
	svc := NewService(repo)
	if err := svc.Delete(context.Background(), "k"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestService_Delete_Error(t *testing.T) {
	repo := repomocks.NewSysConfigRepository(t)
	repo.On("Delete", mock.Anything, "k").Return(errStr("db"))
	svc := NewService(repo)
	if err := svc.Delete(context.Background(), "k"); err == nil {
		t.Error("expected error")
	}
}

func TestService_SeedBuiltins(t *testing.T) {
	repo := repomocks.NewSysConfigRepository(t)
	repo.On("GetAll", mock.Anything).Return([]model.SystemConfig{{Key: "SESSION_TIMEOUT", Value: "24"}}, nil)
	// Only the missing built-in keys get upserted.
	for _, b := range SystemBuiltins() {
		if b.Key == "SESSION_TIMEOUT" {
			continue
		}
		repo.On("Upsert", mock.Anything, b.Key, b.Default).Return(nil).Maybe()
	}
	svc := NewService(repo)
	if err := svc.SeedBuiltins(context.Background()); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	// SESSION_TIMEOUT already exists → must not be overwritten.
	repo.AssertNotCalled(t, "Upsert", mock.Anything, "SESSION_TIMEOUT", mock.Anything)
}

type errStr string

func (e errStr) Error() string { return string(e) }
