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
	repo.On("GetAll", mock.Anything, "models").Return([]model.SystemConfig{{Key: "k", Value: "v"}}, nil)
	svc := NewService(repo)
	cfgs, err := svc.GetAll(context.Background(), "models")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(cfgs) != 1 {
		t.Errorf("cfgs = %v", cfgs)
	}
}

func TestService_GetAll_NilReturnsEmpty(t *testing.T) {
	repo := repomocks.NewSysConfigRepository(t)
	repo.On("GetAll", mock.Anything, "models").Return(([]model.SystemConfig)(nil), nil)
	svc := NewService(repo)
	cfgs, err := svc.GetAll(context.Background(), "models")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(cfgs) != 0 {
		t.Errorf("expected empty slice, got %v", cfgs)
	}
}

func TestService_GetAll_Error(t *testing.T) {
	repo := repomocks.NewSysConfigRepository(t)
	repo.On("GetAll", mock.Anything, "models").Return(([]model.SystemConfig)(nil), errStr("db"))
	svc := NewService(repo)
	_, err := svc.GetAll(context.Background(), "models")
	if err == nil {
		t.Error("expected error")
	}
}

func TestService_Upsert(t *testing.T) {
	repo := repomocks.NewSysConfigRepository(t)
	repo.On("Upsert", mock.Anything, "models", "k", "v").Return(nil)
	svc := NewService(repo)
	if err := svc.Upsert(context.Background(), "models", "k", "v"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

type errStr string

func (e errStr) Error() string { return string(e) }
