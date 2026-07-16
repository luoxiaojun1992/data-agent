package apireview

import (
	"testing"
	"time"
)

func TestStatusConstants(t *testing.T) {
	if StatusPending != "pending" {
		t.Errorf("StatusPending = %q, want pending", StatusPending)
	}
	if StatusApproved != "approved" {
		t.Errorf("StatusApproved = %q, want approved", StatusApproved)
	}
	if StatusRejected != "rejected" {
		t.Errorf("StatusRejected = %q, want rejected", StatusRejected)
	}
}

func TestAPIReviewZeroValue(t *testing.T) {
	r := APIReview{}
	if r.Endpoints != 0 {
		t.Errorf("Endpoints default: got %d, want 0", r.Endpoints)
	}
	if r.Status != "" {
		t.Errorf("Status default: got %q, want empty", r.Status)
	}
}

func TestAPIReviewFieldAssignment(t *testing.T) {
	now := time.Now()
	r := APIReview{
		ID:        "rev-1",
		Name:      "Test API",
		Status:    StatusApproved,
		ReviewedAt: &now,
	}
	if r.ID != "rev-1" {
		t.Errorf("ID: got %s", r.ID)
	}
	if r.Status != StatusApproved {
		t.Errorf("Status: got %s", r.Status)
	}
	if r.ReviewedAt == nil {
		t.Error("ReviewedAt should not be nil")
	}
}
