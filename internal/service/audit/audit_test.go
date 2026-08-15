package audit

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	"github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

func TestNewService(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	userRepo := mocks.NewUserRepository(t)
	s := NewService(repo, userRepo)
	if s == nil {
		t.Fatal("NewService should not return nil")
	}
	if s.repo != repo {
		t.Error("Service.repo should be the injected repository")
	}
	if s.userRepo != userRepo {
		t.Error("Service.userRepo should be the injected user repository")
	}
}

func TestList_Success_NoFilters(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(0), nil)
	repo.On("List", mock.Anything, mock.Anything, int64(0), int64(20)).Return([]model.AuditLog{}, nil)

	result, err := NewService(repo, nil).List(ListParams{})
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
	userRepo := mocks.NewUserRepository(t)

	userRepo.On("SearchByEmail", mock.Anything, "admin", 10).
		Return([]model.User{{ID: "u1", Username: "admin@example.com"}}, nil)
	repo.On("Count", mock.Anything, mock.MatchedBy(hasInFilter("user_id", "u1"))).Return(int64(5), nil)
	repo.On("List", mock.Anything, mock.MatchedBy(hasInFilter("user_id", "u1")), int64(0), int64(50)).Return([]model.AuditLog{}, nil)

	result, err := NewService(repo, userRepo).List(ListParams{
		Action: "login",
		UserID: "admin",
		Start:  "2024-01-01",
		End:    "2024-12-31",
		Skip:   0,
		Limit:  50,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 5 {
		t.Errorf("Total: got %d, want 5", result.Total)
	}
}

func TestList_EmailSearchNoMatch(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	userRepo := mocks.NewUserRepository(t)

	userRepo.On("SearchByEmail", mock.Anything, "nobody", 10).Return([]model.User{}, nil)

	result, err := NewService(repo, userRepo).List(ListParams{UserID: "nobody"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("Total: got %d, want 0", result.Total)
	}
	if len(result.Logs) != 0 {
		t.Errorf("Logs: got %d, want 0", len(result.Logs))
	}
	// Count/List must NOT be called when no users match.
	repo.AssertNotCalled(t, "Count", mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "List", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestList_EnrichEmails(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	userRepo := mocks.NewUserRepository(t)

	logs := []model.AuditLog{
		{ID: "l1", UserID: "u1"},
		{ID: "l2", UserID: "u2"},
		{ID: "l3", UserID: "u1"},
	}
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(3), nil)
	repo.On("List", mock.Anything, mock.Anything, int64(0), int64(20)).Return(logs, nil)
	userRepo.On("FindByIDs", mock.Anything, mock.MatchedBy(idsContain("u1", "u2"))).
		Return([]model.User{
			{ID: "u1", Username: "a@example.com"},
			{ID: "u2", Username: "b@example.com"},
		}, nil)

	result, err := NewService(repo, userRepo).List(ListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Logs[0].UserID != "a@example.com" {
		t.Errorf("log[0] user: got %q, want email", result.Logs[0].UserID)
	}
	if result.Logs[1].UserID != "b@example.com" {
		t.Errorf("log[1] user: got %q, want email", result.Logs[1].UserID)
	}
	if result.Logs[2].UserID != "a@example.com" {
		t.Errorf("log[2] user: got %q, want email", result.Logs[2].UserID)
	}
}

func TestList_EnrichEmails_MissingUserKeepsID(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	userRepo := mocks.NewUserRepository(t)

	logs := []model.AuditLog{{ID: "l1", UserID: "ghost"}}
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(1), nil)
	repo.On("List", mock.Anything, mock.Anything, int64(0), int64(20)).Return(logs, nil)
	// Deleted user → FindByIDs returns nothing for that ID.
	userRepo.On("FindByIDs", mock.Anything, mock.Anything).Return([]model.User{}, nil)

	result, err := NewService(repo, userRepo).List(ListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Logs[0].UserID != "ghost" {
		t.Errorf("missing user should keep original ID: got %q", result.Logs[0].UserID)
	}
}

func TestList_EnrichEmails_LookupErrorNonFatal(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	userRepo := mocks.NewUserRepository(t)

	logs := []model.AuditLog{{ID: "l1", UserID: "u1"}}
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(1), nil)
	repo.On("List", mock.Anything, mock.Anything, int64(0), int64(20)).Return(logs, nil)
	userRepo.On("FindByIDs", mock.Anything, mock.Anything).Return(nil, errors.New("db down"))

	result, err := NewService(repo, userRepo).List(ListParams{})
	if err != nil {
		t.Fatalf("enrich failure must be non-fatal: %v", err)
	}
	if result.Logs[0].UserID != "u1" {
		t.Errorf("lookup failure should keep original ID: got %q", result.Logs[0].UserID)
	}
}

func TestList_EmailSearchError(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	userRepo := mocks.NewUserRepository(t)
	userRepo.On("SearchByEmail", mock.Anything, "admin", 10).Return(nil, errors.New("search failed"))

	_, err := NewService(repo, userRepo).List(ListParams{UserID: "admin"})
	if err == nil {
		t.Fatal("expected error for email search failure")
	}
}

func TestList_CountError(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(0), errors.New("count failed"))

	_, err := NewService(repo, nil).List(ListParams{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestList_FindError(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(0), nil)
	repo.On("List", mock.Anything, mock.Anything, int64(0), int64(20)).Return(([]model.AuditLog)(nil), errors.New("list failed"))

	_, err := NewService(repo, nil).List(ListParams{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestList_DefaultLimit(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(0), nil)
	repo.On("List", mock.Anything, mock.Anything, int64(0), int64(20)).Return([]model.AuditLog{}, nil)

	result, err := NewService(repo, nil).List(ListParams{Limit: 0})
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

	result, err := NewService(repo, nil).List(ListParams{Limit: 500})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil ListResult")
	}
}

func TestList_InvalidStartDate(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	_, err := NewService(repo, nil).List(ListParams{Start: "invalid-date"})
	if err == nil {
		t.Fatal("expected error for invalid start date")
	}
}

func TestList_InvalidEndDate(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	_, err := NewService(repo, nil).List(ListParams{End: "invalid-date"})
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

// hasInFilter asserts a filter map has `field` set to {$in: [...]} containing want.
func hasInFilter(field, want string) func(map[string]interface{}) bool {
	return func(m map[string]interface{}) bool {
		v, ok := m[field]
		if !ok {
			return false
		}
		inner, ok := v.(map[string]interface{})
		if !ok {
			return false
		}
		in, ok := inner["$in"].([]string)
		if !ok {
			return false
		}
		for _, s := range in {
			if s == want {
				return true
			}
		}
		return false
	}
}

// idsContain asserts the []string arg contains all wanted IDs.
func idsContain(want ...string) func([]string) bool {
	return func(ids []string) bool {
		seen := map[string]bool{}
		for _, id := range ids {
			seen[id] = true
		}
		for _, w := range want {
			if !seen[w] {
				return false
			}
		}
		return true
	}
}

// ensure repository.UserRepository import is used (kept for interface clarity).
var _ repository.UserRepository = (*mocks.UserRepository)(nil)
