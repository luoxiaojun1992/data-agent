package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/gin-gonic/gin"
	apireview "github.com/luoxiaojun1992/data-agent/internal/domain/apireview"
	mockapireview "github.com/luoxiaojun1992/data-agent/internal/service/apireview/mocks"
)

func init() { gin.SetMode(gin.TestMode) }

// ── NewAPIReviewHandler ──

func TestNewAPIReviewHandler(t *testing.T) {
	svc := mockapireview.NewAPIReviewService(t)
	h := NewAPIReviewHandler(svc)
	if h == nil {
		t.Fatal("NewAPIReviewHandler returned nil")
	}
	if h.svc == nil {
		t.Error("svc not set correctly")
	}
}

// ── ListAPIReviews ──

func TestListAPIReviews_Success(t *testing.T) {
	svc := mockapireview.NewAPIReviewService(t)
	h := NewAPIReviewHandler(svc)

	now := time.Now()
	reviews := []apireview.APIReview{
		{ID: "apirev_1", Name: "Review 1", Status: apireview.StatusPending, CreatedAt: now, UpdatedAt: now},
		{ID: "apirev_2", Name: "Review 2", Status: apireview.StatusApproved, CreatedAt: now, UpdatedAt: now},
	}
	svc.On("ListAll").Return(mock.Anything).Return( reviews, nil)

	c, w := newGinContext("GET", "/reviews", "")
	h.ListAPIReviews(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result []apireview.APIReview
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 reviews, got %d", len(result))
	}
}

func TestListAPIReviews_Empty(t *testing.T) {
	svc := mockapireview.NewAPIReviewService(t)
	h := NewAPIReviewHandler(svc)

	svc.On("ListAll").Return(mock.Anything).Return( []apireview.APIReview{}, nil)

	c, w := newGinContext("GET", "/reviews", "")
	h.ListAPIReviews(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []apireview.APIReview
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 0 {
		t.Errorf("expected 0 reviews, got %d", len(result))
	}
}

func TestListAPIReviews_Error(t *testing.T) {
	svc := mockapireview.NewAPIReviewService(t)
	h := NewAPIReviewHandler(svc)

	svc.On("ListAll").Return(mock.Anything).Return( nil, fmt.Errorf("db error"))

	c, w := newGinContext("GET", "/reviews", "")
	h.ListAPIReviews(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── CreateAPIReview ──

func TestCreateAPIReview_Success(t *testing.T) {
	svc := mockapireview.NewAPIReviewService(t)
	h := NewAPIReviewHandler(svc)

	now := time.Now()
	mockReview := &apireview.APIReview{
		ID:        "apirev_new",
		Name:      "Test API",
		FileName:  "test.proto",
		Domain:    "example.com",
		Version:   "3.0",
		Endpoints: 10,
		RateLimit: 100,
		Submitter: "user-1",
		Status:    apireview.StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}

	svc.On("Create", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mock.Anything).Return( mockReview, nil)

	body := `{"name":"Test API","file_name":"test.proto","domain":"example.com","version":"3.0","endpoints":10,"rate_limit":100}`
	c, w := newGinContext("POST", "/reviews", body)
	c.Set("user_id", "user-1")
	h.CreateAPIReview(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var result apireview.APIReview
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.ID != "apirev_new" {
		t.Errorf("ID: got %s", result.ID)
	}
	if result.Name != "Test API" {
		t.Errorf("Name: got %s", result.Name)
	}
}

func TestCreateAPIReview_DefaultVersion(t *testing.T) {
	svc := mockapireview.NewAPIReviewService(t)
	h := NewAPIReviewHandler(svc)

	mockReview := &apireview.APIReview{
		ID:        "apirev_default",
		Name:      "No Version API",
		Version:   "3.0",
		Submitter: "user-1",
	}

	svc.On("Create", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mock.Anything).Return( mockReview, nil)

	body := `{"name":"No Version API","file_name":"test.proto"}`
	c, w := newGinContext("POST", "/reviews", body)
	c.Set("user_id", "user-1")
	h.CreateAPIReview(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateAPIReview_MissingRequired(t *testing.T) {
	svc := mockapireview.NewAPIReviewService(t)
	h := NewAPIReviewHandler(svc)

	body := `{}`
	c, w := newGinContext("POST", "/reviews", body)
	c.Set("user_id", "user-1")
	h.CreateAPIReview(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateAPIReview_MissingFileName(t *testing.T) {
	svc := mockapireview.NewAPIReviewService(t)
	h := NewAPIReviewHandler(svc)

	body := `{"name":"Test"}`
	c, w := newGinContext("POST", "/reviews", body)
	c.Set("user_id", "user-1")
	h.CreateAPIReview(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateAPIReview_InvalidJSON(t *testing.T) {
	svc := mockapireview.NewAPIReviewService(t)
	h := NewAPIReviewHandler(svc)

	c, w := newGinContext("POST", "/reviews", "not-json")
	c.Set("user_id", "user-1")
	h.CreateAPIReview(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateAPIReview_ServiceError(t *testing.T) {
	svc := mockapireview.NewAPIReviewService(t)
	h := NewAPIReviewHandler(svc)

	svc.On("Create", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mock.Anything).Return( nil, fmt.Errorf("insert failed"))

	body := `{"name":"Test API","file_name":"test.proto"}`
	c, w := newGinContext("POST", "/reviews", body)
	c.Set("user_id", "user-1")
	h.CreateAPIReview(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── ApproveAPIReview ──

func TestApproveAPIReview_Success(t *testing.T) {
	svc := mockapireview.NewAPIReviewService(t)
	h := NewAPIReviewHandler(svc)

	svc.On("Approve", mock.Anything, mock.Anything).Return(mock.Anything).Return( nil)

	c, w := newGinContext("POST", "/reviews/apirev_1/approve", "")
	c.Set("user_id", "reviewer-1")
	c.Params = gin.Params{{Key: "id", Value: "apirev_1"}}
	h.ApproveAPIReview(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"approved"`) {
		t.Errorf("body should contain approved: %s", w.Body.String())
	}
}

func TestApproveAPIReview_Error(t *testing.T) {
	svc := mockapireview.NewAPIReviewService(t)
	h := NewAPIReviewHandler(svc)

	svc.On("Approve", mock.Anything, mock.Anything).Return(mock.Anything).Return( fmt.Errorf("cannot approve own"))

	c, w := newGinContext("POST", "/reviews/apirev_1/approve", "")
	c.Set("user_id", "user-1")
	c.Params = gin.Params{{Key: "id", Value: "apirev_1"}}
	h.ApproveAPIReview(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestApproveAPIReview_NotFound(t *testing.T) {
	svc := mockapireview.NewAPIReviewService(t)
	h := NewAPIReviewHandler(svc)

	svc.On("Approve", mock.Anything, mock.Anything).Return(mock.Anything).Return( fmt.Errorf("find api review: not found"))

	c, w := newGinContext("POST", "/reviews/nonexistent/approve", "")
	c.Set("user_id", "reviewer-1")
	c.Params = gin.Params{{Key: "id", Value: "nonexistent"}}
	h.ApproveAPIReview(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── RejectAPIReview ──

func TestRejectAPIReview_Success(t *testing.T) {
	svc := mockapireview.NewAPIReviewService(t)
	h := NewAPIReviewHandler(svc)

	svc.On("Reject", mock.Anything, mock.Anything, mock.Anything).Return(mock.Anything).Return( nil)

	body := `{"reason":"Needs more work"}`
	c, w := newGinContext("POST", "/reviews/apirev_1/reject", body)
	c.Set("user_id", "reviewer-1")
	c.Params = gin.Params{{Key: "id", Value: "apirev_1"}}
	h.RejectAPIReview(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"rejected"`) {
		t.Errorf("body should contain rejected: %s", w.Body.String())
	}
}

func TestRejectAPIReview_MissingReason(t *testing.T) {
	svc := mockapireview.NewAPIReviewService(t)
	h := NewAPIReviewHandler(svc)

	body := `{}`
	c, w := newGinContext("POST", "/reviews/apirev_1/reject", body)
	c.Set("user_id", "reviewer-1")
	c.Params = gin.Params{{Key: "id", Value: "apirev_1"}}
	h.RejectAPIReview(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRejectAPIReview_InvalidJSON(t *testing.T) {
	svc := mockapireview.NewAPIReviewService(t)
	h := NewAPIReviewHandler(svc)

	c, w := newGinContext("POST", "/reviews/apirev_1/reject", "bad")
	c.Set("user_id", "reviewer-1")
	c.Params = gin.Params{{Key: "id", Value: "apirev_1"}}
	h.RejectAPIReview(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRejectAPIReview_ServiceError(t *testing.T) {
	svc := mockapireview.NewAPIReviewService(t)
	h := NewAPIReviewHandler(svc)

	svc.On("Reject", mock.Anything, mock.Anything, mock.Anything).Return(mock.Anything).Return( fmt.Errorf("only pending can be rejected"))

	body := `{"reason":"already approved"}`
	c, w := newGinContext("POST", "/reviews/apirev_1/reject", body)
	c.Set("user_id", "reviewer-1")
	c.Params = gin.Params{{Key: "id", Value: "apirev_1"}}
	h.RejectAPIReview(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
