package apireview

import (
	"context"
	"errors"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/luoxiaojun1992/data-agent/internal/domain/apireview"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestGenShortID(t *testing.T) {
	id := genShortID()
	if id == "" {
		t.Error("genShortID should not return empty string")
	}
	if len(id) != 12 {
		t.Errorf("genShortID length: got %d, want 12", len(id))
	}
}

func TestNewService(t *testing.T) {
	db := &mongo.Database{}
	var coll mongo.Collection
	patches := gomonkey.ApplyMethodReturn(db, "Collection", &coll)
	defer patches.Reset()

	s := NewService(mongoinfra.NewAPIReviewRepository(db))
	if s == nil {
		t.Fatal("NewService should not return nil")
	}
	if s.coll != &coll {
		t.Error("Service.coll should be the Collection returned by db.Collection")
	}
}

func TestCreate_Success(t *testing.T) {
	var coll mongo.Collection
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "InsertOne", &mongo.InsertOneResult{}, nil)

	svc := &Service{coll: nil // coll removed}
	r, err := svc.Create("test-api", "test.json", "example.com", "3.0", 10, 100, "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil APIReview")
	}
	if r.Name != "test-api" {
		t.Errorf("Name: got %q, want %q", r.Name, "test-api")
	}
	if r.FileName != "test.json" {
		t.Errorf("FileName: got %q, want %q", r.FileName, "test.json")
	}
	if r.Domain != "example.com" {
		t.Errorf("Domain: got %q, want %q", r.Domain, "example.com")
	}
	if r.Version != "3.0" {
		t.Errorf("Version: got %q, want %q", r.Version, "3.0")
	}
	if r.Endpoints != 10 {
		t.Errorf("Endpoints: got %d, want 10", r.Endpoints)
	}
	if r.RateLimit != 100 {
		t.Errorf("RateLimit: got %d, want 100", r.RateLimit)
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
}

func TestCreate_InsertError(t *testing.T) {
	var coll mongo.Collection
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "InsertOne", (*mongo.InsertOneResult)(nil), errors.New("insert failed"))

	svc := &Service{coll: nil // coll removed}
	_, err := svc.Create("test-api", "test.json", "example.com", "3.0", 10, 100, "user1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListAll_Success(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodFunc(&cur, "All", func(ctx context.Context, results interface{}) error {
		slice := results.(*[]apireview.APIReview)
		*slice = []apireview.APIReview{
			{ID: "r1", Name: "API 1", Status: apireview.StatusPending},
			{ID: "r2", Name: "API 2", Status: apireview.StatusApproved},
		}
		return nil
	})

	svc := &Service{coll: nil // coll removed}
	reviews, err := svc.ListAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reviews) != 2 {
		t.Fatalf("expected 2 reviews, got %d", len(reviews))
	}
	if reviews[0].ID != "r1" {
		t.Errorf("reviews[0].ID: got %q, want r1", reviews[0].ID)
	}
}

func TestListAll_FindError(t *testing.T) {
	var coll mongo.Collection
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", (*mongo.Cursor)(nil), errors.New("find failed"))

	svc := &Service{coll: nil // coll removed}
	_, err := svc.ListAll()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListAll_CursorAllError(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodReturn(&cur, "All", errors.New("cursor all failed"))

	svc := &Service{coll: nil // coll removed}
	_, err := svc.ListAll()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestApprove_Success(t *testing.T) {
	var coll mongo.Collection
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()

	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodFunc(&sr, "Decode", func(v interface{}) error {
		r := v.(*apireview.APIReview)
		r.ID = "apirev_test1234"
		r.Name = "Test API"
		r.Status = apireview.StatusPending
		r.Submitter = "user1"
		return nil
	})
	patches.ApplyMethodFunc(&coll, "UpdateOne", func(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
		u := update.(bson.M)
		setOp := u["$set"].(bson.M)
		if setOp["status"] != apireview.StatusApproved {
			t.Errorf("Approve status: got %v, want %q", setOp["status"], apireview.StatusApproved)
		}
		if setOp["reviewer"] != "reviewer1" {
			t.Errorf("Approve reviewer: got %v, want reviewer1", setOp["reviewer"])
		}
		return &mongo.UpdateResult{}, nil
	})

	svc := &Service{coll: nil // coll removed}
	err := svc.Approve("apirev_test1234", "reviewer1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApprove_FindError(t *testing.T) {
	var coll mongo.Collection
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodReturn(&sr, "Decode", mongo.ErrNoDocuments)

	svc := &Service{coll: nil // coll removed}
	err := svc.Approve("apirev_test1234", "reviewer1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestApprove_NotPending(t *testing.T) {
	var coll mongo.Collection
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodFunc(&sr, "Decode", func(v interface{}) error {
		r := v.(*apireview.APIReview)
		r.ID = "apirev_test1234"
		r.Status = apireview.StatusApproved
		r.Submitter = "user1"
		return nil
	})

	svc := &Service{coll: nil // coll removed}
	err := svc.Approve("apirev_test1234", "reviewer1")
	if err == nil {
		t.Fatal("expected error for non-pending review")
	}
}

func TestApprove_SelfReview(t *testing.T) {
	var coll mongo.Collection
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodFunc(&sr, "Decode", func(v interface{}) error {
		r := v.(*apireview.APIReview)
		r.ID = "apirev_test1234"
		r.Status = apireview.StatusPending
		r.Submitter = "reviewer1"
		return nil
	})

	svc := &Service{coll: nil // coll removed}
	err := svc.Approve("apirev_test1234", "reviewer1")
	if err == nil {
		t.Fatal("expected error for self-review")
	}
}

func TestApprove_UpdateError(t *testing.T) {
	var coll mongo.Collection
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodFunc(&sr, "Decode", func(v interface{}) error {
		r := v.(*apireview.APIReview)
		r.ID = "apirev_test1234"
		r.Status = apireview.StatusPending
		r.Submitter = "user1"
		return nil
	})
	patches.ApplyMethodReturn(&coll, "UpdateOne", (*mongo.UpdateResult)(nil), errors.New("update failed"))

	svc := &Service{coll: nil // coll removed}
	err := svc.Approve("apirev_test1234", "reviewer1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReject_Success(t *testing.T) {
	var coll mongo.Collection
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodFunc(&sr, "Decode", func(v interface{}) error {
		r := v.(*apireview.APIReview)
		r.ID = "apirev_test1234"
		r.Status = apireview.StatusPending
		return nil
	})
	patches.ApplyMethodFunc(&coll, "UpdateOne", func(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
		u := update.(bson.M)
		setOp := u["$set"].(bson.M)
		if setOp["status"] != apireview.StatusRejected {
			t.Errorf("Reject status: got %v, want %q", setOp["status"], apireview.StatusRejected)
		}
		if setOp["reviewer"] != "reviewer1" {
			t.Errorf("Reject reviewer: got %v, want reviewer1", setOp["reviewer"])
		}
		if setOp["reject_reason"] != "not good enough" {
			t.Errorf("Reject reason: got %v, want 'not good enough'", setOp["reject_reason"])
		}
		return &mongo.UpdateResult{}, nil
	})

	svc := &Service{coll: nil // coll removed}
	err := svc.Reject("apirev_test1234", "reviewer1", "not good enough")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReject_EmptyReason(t *testing.T) {
	svc := &Service{coll: nil}
	err := svc.Reject("apirev_test1234", "reviewer1", "")
	if err == nil {
		t.Fatal("expected error for empty reason")
	}
}

func TestReject_FindError(t *testing.T) {
	var coll mongo.Collection
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodReturn(&sr, "Decode", mongo.ErrNoDocuments)

	svc := &Service{coll: nil // coll removed}
	err := svc.Reject("apirev_test1234", "reviewer1", "reason")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReject_NotPending(t *testing.T) {
	var coll mongo.Collection
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodFunc(&sr, "Decode", func(v interface{}) error {
		r := v.(*apireview.APIReview)
		r.ID = "apirev_test1234"
		r.Status = apireview.StatusRejected
		return nil
	})

	svc := &Service{coll: nil // coll removed}
	err := svc.Reject("apirev_test1234", "reviewer1", "reason")
	if err == nil {
		t.Fatal("expected error for non-pending review")
	}
}

func TestReject_UpdateError(t *testing.T) {
	var coll mongo.Collection
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodFunc(&sr, "Decode", func(v interface{}) error {
		r := v.(*apireview.APIReview)
		r.ID = "apirev_test1234"
		r.Status = apireview.StatusPending
		return nil
	})
	patches.ApplyMethodReturn(&coll, "UpdateOne", (*mongo.UpdateResult)(nil), errors.New("update failed"))

	svc := &Service{coll: nil // coll removed}
	err := svc.Reject("apirev_test1234", "reviewer1", "reason")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListAll_NilReviews(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodFunc(&cur, "All", func(ctx context.Context, results interface{}) error {
		return nil
	})

	svc := &Service{coll: nil // coll removed}
	reviews, err := svc.ListAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reviews == nil || len(reviews) != 0 {
		t.Errorf("expected non-nil empty slice, got %v (len=%d)", reviews, len(reviews))
	}
}

// Ensure bson is used
var _ = bson.M{}
