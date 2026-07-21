package apireview

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/apireview"
	"github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

func TestGenShortID(t *testing.T) {
	id := genShortID()
	if id == "" {
		t.Error("genShortID should not return empty string")
	}
	if len(id) != 8 {
		t.Errorf("genShortID length: got %d, want 8", len(id))
	}
}

func TestNewService(t *testing.T) {
	repo := mocks.NewAPIReviewRepository(t)
	s := NewService(repo)
	if s == nil {
		t.Fatal("NewService should not return nil")
	}
}

func TestCreate_Success(t *testing.T) {
	repo := mocks.NewAPIReviewRepository(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)

	r, err := NewService(repo).Create("test-api", "test.json", "example.com", "3.0", 10, 100, "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil APIReview")
	}
	if r.Name != "test-api" {
		t.Errorf("Name: got %q, want %q", r.Name, "test-api")
	}
	if r.Status != apireview.StatusPending {
		t.Errorf("Status: got %q, want %q", r.Status, apireview.StatusPending)
	}
	if r.Submitter != "user1" {
		t.Errorf("Submitter: got %q, want %q", r.Submitter, "user1")
	}
	if r.ID == "" {
		t.Error("ID should not be empty")
	}
	// repo.Create received the constructed review (verified by mock assertion).
	repo.AssertCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestCreate_InsertError(t *testing.T) {
	repo := mocks.NewAPIReviewRepository(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(errors.New("insert failed"))

	_, err := NewService(repo).Create("test-api", "test.json", "example.com", "3.0", 10, 100, "user1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListAll_Success(t *testing.T) {
	repo := mocks.NewAPIReviewRepository(t)
	repo.On("List", mock.Anything, int64(0), int64(100)).Return([]apireview.APIReview{
		{ID: "r1", Name: "API 1", Status: apireview.StatusPending},
		{ID: "r2", Name: "API 2", Status: apireview.StatusApproved},
	}, nil)

	reviews, err := NewService(repo).ListAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reviews) != 2 {
		t.Fatalf("expected 2 reviews, got %d", len(reviews))
	}
	if reviews[0].Name != "API 1" {
		t.Errorf("reviews[0].Name: got %q, want %q", reviews[0].Name, "API 1")
	}
	if reviews[1].Status != apireview.StatusApproved {
		t.Errorf("reviews[1].Status: got %q, want %q", reviews[1].Status, apireview.StatusApproved)
	}
}

func TestListAll_FindError(t *testing.T) {
	repo := mocks.NewAPIReviewRepository(t)
	repo.On("List", mock.Anything, int64(0), int64(100)).Return(([]apireview.APIReview)(nil), errors.New("list failed"))

	_, err := NewService(repo).ListAll()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListAll_NilReviews(t *testing.T) {
	repo := mocks.NewAPIReviewRepository(t)
	repo.On("List", mock.Anything, int64(0), int64(100)).Return(([]apireview.APIReview)(nil), nil)

	reviews, err := NewService(repo).ListAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reviews == nil || len(reviews) != 0 {
		t.Errorf("expected non-nil empty slice, got len=%d", len(reviews))
	}
}

func TestApprove_Success(t *testing.T) {
	repo := mocks.NewAPIReviewRepository(t)
	repo.On("FindByID", mock.Anything, "apirev_test1234").Return(&apireview.APIReview{
		ID: "apirev_test1234", Status: apireview.StatusPending, Submitter: "user1",
	}, nil)
	repo.On("UpdateStatus", mock.Anything, "apirev_test1234", mock.Anything).Return(nil)

	err := NewService(repo).Approve("apirev_test1234", "reviewer1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApprove_FindError(t *testing.T) {
	repo := mocks.NewAPIReviewRepository(t)
	repo.On("FindByID", mock.Anything, "apirev_test1234").Return((*apireview.APIReview)(nil), errors.New("not found"))

	err := NewService(repo).Approve("apirev_test1234", "reviewer1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestApprove_NotFound(t *testing.T) {
	repo := mocks.NewAPIReviewRepository(t)
	repo.On("FindByID", mock.Anything, "apirev_test1234").Return((*apireview.APIReview)(nil), nil)

	err := NewService(repo).Approve("apirev_test1234", "reviewer1")
	if err == nil {
		t.Fatal("expected error for missing review")
	}
}

func TestApprove_NotPending(t *testing.T) {
	repo := mocks.NewAPIReviewRepository(t)
	repo.On("FindByID", mock.Anything, "apirev_test1234").Return(&apireview.APIReview{
		ID: "apirev_test1234", Status: apireview.StatusApproved, Submitter: "user1",
	}, nil)

	err := NewService(repo).Approve("apirev_test1234", "reviewer1")
	if err == nil {
		t.Fatal("expected error for non-pending review")
	}
}

func TestApprove_SelfReview(t *testing.T) {
	repo := mocks.NewAPIReviewRepository(t)
	repo.On("FindByID", mock.Anything, "apirev_test1234").Return(&apireview.APIReview{
		ID: "apirev_test1234", Status: apireview.StatusPending, Submitter: "reviewer1",
	}, nil)

	err := NewService(repo).Approve("apirev_test1234", "reviewer1")
	if err == nil {
		t.Fatal("expected error for self-review")
	}
}

func TestApprove_UpdateError(t *testing.T) {
	repo := mocks.NewAPIReviewRepository(t)
	repo.On("FindByID", mock.Anything, "apirev_test1234").Return(&apireview.APIReview{
		ID: "apirev_test1234", Status: apireview.StatusPending, Submitter: "user1",
	}, nil)
	repo.On("UpdateStatus", mock.Anything, "apirev_test1234", mock.Anything).Return(errors.New("update failed"))

	err := NewService(repo).Approve("apirev_test1234", "reviewer1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReject_Success(t *testing.T) {
	repo := mocks.NewAPIReviewRepository(t)
	repo.On("FindByID", mock.Anything, "apirev_test1234").Return(&apireview.APIReview{
		ID: "apirev_test1234", Status: apireview.StatusPending, Submitter: "submitter1",
	}, nil)
	repo.On("UpdateStatus", mock.Anything, "apirev_test1234", mock.Anything).Return(nil)

	err := NewService(repo).Reject("apirev_test1234", "reviewer1", "not good enough")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReject_EmptyReason(t *testing.T) {
	repo := mocks.NewAPIReviewRepository(t)
	err := NewService(repo).Reject("apirev_test1234", "reviewer1", "")
	if err == nil {
		t.Fatal("expected error for empty reason")
	}
}

func TestReject_FindError(t *testing.T) {
	repo := mocks.NewAPIReviewRepository(t)
	repo.On("FindByID", mock.Anything, "apirev_test1234").Return((*apireview.APIReview)(nil), errors.New("not found"))

	err := NewService(repo).Reject("apirev_test1234", "reviewer1", "reason")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReject_NotPending(t *testing.T) {
	repo := mocks.NewAPIReviewRepository(t)
	repo.On("FindByID", mock.Anything, "apirev_test1234").Return(&apireview.APIReview{
		ID: "apirev_test1234", Status: apireview.StatusRejected,
	}, nil)

	err := NewService(repo).Reject("apirev_test1234", "reviewer1", "reason")
	if err == nil {
		t.Fatal("expected error for non-pending review")
	}
}

func TestReject_UpdateError(t *testing.T) {
	repo := mocks.NewAPIReviewRepository(t)
	repo.On("FindByID", mock.Anything, "apirev_test1234").Return(&apireview.APIReview{
		ID: "apirev_test1234", Status: apireview.StatusPending,
	}, nil)
	repo.On("UpdateStatus", mock.Anything, "apirev_test1234", mock.Anything).Return(errors.New("update failed"))

	err := NewService(repo).Reject("apirev_test1234", "reviewer1", "reason")
	if err == nil {
		t.Fatal("expected error")
	}
}
