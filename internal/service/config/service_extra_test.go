package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	repomocks "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

// List has real pagination logic (page/pageSize clamping, skip calculation)
// that the existing tests never reached.

func TestService_List_SkipCalculation(t *testing.T) {
	repo := repomocks.NewSysConfigRepository(t)
	// page=2, pageSize=10 → skip=(2-1)*10=10, limit=10
	repo.On("List", mock.Anything, int64(10), int64(10)).Return([]model.SystemConfig{{Key: "k", Value: "v"}}, nil)
	repo.On("Count", mock.Anything).Return(int64(1), nil)
	svc := NewService(repo)

	cfgs, total, err := svc.List(context.Background(), 2, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfgs) != 1 || total != 1 {
		t.Errorf("cfgs=%v total=%d", cfgs, total)
	}
	repo.AssertCalled(t, "List", mock.Anything, int64(10), int64(10))
}

func TestService_List_ClampsPage(t *testing.T) {
	repo := repomocks.NewSysConfigRepository(t)
	// page<1 → 1 (skip 0); pageSize<1 → 20
	repo.On("List", mock.Anything, int64(0), int64(20)).Return([]model.SystemConfig{}, nil)
	repo.On("Count", mock.Anything).Return(int64(0), nil)
	svc := NewService(repo)

	_, _, err := svc.List(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	repo.AssertCalled(t, "List", mock.Anything, int64(0), int64(20))
}

func TestService_List_ClampsPageSizeHigh(t *testing.T) {
	repo := repomocks.NewSysConfigRepository(t)
	// pageSize>100 → 20
	repo.On("List", mock.Anything, int64(0), int64(20)).Return([]model.SystemConfig{}, nil)
	repo.On("Count", mock.Anything).Return(int64(0), nil)
	svc := NewService(repo)

	_, _, err := svc.List(context.Background(), 1, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	repo.AssertCalled(t, "List", mock.Anything, int64(0), int64(20))
}

func TestService_List_ListError(t *testing.T) {
	repo := repomocks.NewSysConfigRepository(t)
	repo.On("List", mock.Anything, mock.Anything, mock.Anything).Return(([]model.SystemConfig)(nil), errStr("db"))
	svc := NewService(repo)
	if _, _, err := svc.List(context.Background(), 1, 10); err == nil {
		t.Error("expected error from List")
	}
}

func TestService_List_CountError(t *testing.T) {
	repo := repomocks.NewSysConfigRepository(t)
	repo.On("List", mock.Anything, mock.Anything, mock.Anything).Return([]model.SystemConfig{}, nil)
	repo.On("Count", mock.Anything).Return(int64(0), errStr("db"))
	svc := NewService(repo)
	if _, _, err := svc.List(context.Background(), 1, 10); err == nil {
		t.Error("expected error from Count")
	}
}

func TestService_SeedBuiltins_GetAllError(t *testing.T) {
	repo := repomocks.NewSysConfigRepository(t)
	repo.On("GetAll", mock.Anything).Return(([]model.SystemConfig)(nil), errStr("db"))
	svc := NewService(repo)
	if err := svc.SeedBuiltins(context.Background()); err == nil {
		t.Error("expected error when GetAll fails during seed")
	}
}

func TestService_SeedBuiltins_DropsEmptyKeyRow(t *testing.T) {
	repo := repomocks.NewSysConfigRepository(t)
	// a legacy empty-key row must be deleted and not treated as a builtin.
	repo.On("GetAll", mock.Anything).Return([]model.SystemConfig{
		{ID: "legacy", Key: "", Value: "orphan"},
		{Key: "SESSION_TIMEOUT", Value: "24"},
	}, nil)
	repo.On("Delete", mock.Anything, "").Return(nil).Once()
	for _, b := range SystemBuiltins() {
		if b.Key == "SESSION_TIMEOUT" {
			repo.On("Upsert", mock.Anything, b.Key, "24", b.Description).Return(nil).Once()
			continue
		}
		repo.On("Upsert", mock.Anything, b.Key, b.Default, b.Description).Return(nil).Maybe()
	}
	svc := NewService(repo)
	if err := svc.SeedBuiltins(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	repo.AssertCalled(t, "Delete", mock.Anything, "")
	repo.AssertCalled(t, "Upsert", mock.Anything, "SESSION_TIMEOUT", "24", "登录 Session 超时（小时）")
}
