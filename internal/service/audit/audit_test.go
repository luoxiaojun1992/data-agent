package audit

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

func TestNewService(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	s := NewService(repo)
	if s == nil {
		t.Fatal("NewService should not return nil")
	}
	if s.repo != repo {
		t.Error("Service.repo should be the injected repository")
	}
}

func TestList_Success_NoFilters(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(0), nil)
	repo.On("List", mock.Anything, mock.Anything, int64(0), int64(20)).Return([]model.AuditLog{}, nil)

	result, err := NewService(repo).List(ListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil ListResult")
	}
	if result.Logs == nil {
		t.Error("expected non-nil Logs slice")
	}
	if result.Total != 0 {
		t.Errorf("Total: got %d, want 0", result.Total)
	}
}

func TestList_Success_WithFilters(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(5), nil)
	repo.On("List", mock.Anything, mock.Anything, int64(0), int64(50)).Return([]model.AuditLog{}, nil)

	result, err := NewService(repo).List(ListParams{
		Action: "login",
		UserID: "user1",
		Start:  "2024-01-01",
		End:    "2024-12-31",
		Skip:   0,
		Limit:  50,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil ListResult")
	}
	if result.Total != 5 {
		t.Errorf("Total: got %d, want 5", result.Total)
	}
}

func TestList_CountError(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(0), errors.New("count failed"))

	_, err := NewService(repo).List(ListParams{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestList_FindError(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(0), nil)
	repo.On("List", mock.Anything, mock.Anything, int64(0), int64(20)).Return(([]model.AuditLog)(nil), errors.New("list failed"))

	_, err := NewService(repo).List(ListParams{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestList_DefaultLimit(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(0), nil)
	repo.On("List", mock.Anything, mock.Anything, int64(0), int64(20)).Return([]model.AuditLog{}, nil)

	result, err := NewService(repo).List(ListParams{Limit: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil ListResult")
	}
}

func TestList_LimitCapped(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(0), nil)
	repo.On("List", mock.Anything, mock.Anything, int64(0), int64(100)).Return([]model.AuditLog{}, nil)

	result, err := NewService(repo).List(ListParams{Limit: 500})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil ListResult")
	}
}

func TestList_InvalidStartDate(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	_, err := NewService(repo).List(ListParams{Start: "invalid-date"})
	if err == nil {
		t.Fatal("expected error for invalid start date")
	}
}

func TestList_InvalidEndDate(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	_, err := NewService(repo).List(ListParams{End: "invalid-date"})
	if err == nil {
		t.Fatal("expected error for invalid end date")
	}
}

func TestListResult_EmptyState(t *testing.T) {
	r := &ListResult{Logs: nil, Total: 0}
	if r.Total != 0 {
		t.Errorf("Total: got %d, want 0", r.Total)
	}
}
