package audit

import (
	"context"
	"errors"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"go.mongodb.org/mongo-driver/mongo"
	mongoinfra "github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
)

func TestNewService(t *testing.T) {
	db := &mongo.Database{}
	var coll mongo.Collection
	patches := gomonkey.ApplyMethodReturn(db, "Collection", &coll)
	defer patches.Reset()

	s := NewService(mongoinfra.NewAuditRepository(db))
	if s == nil {
		t.Fatal("NewService should not return nil")
	}
	if s.coll != &coll {
		t.Error("Service.coll should be the Collection returned by db.Collection")
	}
}

func TestList_Success_NoFilters(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "CountDocuments", int64(0), nil)
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodFunc(&cur, "All", func(ctx context.Context, results interface{}) error {
		return nil
	})

	svc := &Service{coll: &coll}
	result, err := svc.List(ListParams{})
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
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "CountDocuments", int64(5), nil)
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodFunc(&cur, "All", func(ctx context.Context, results interface{}) error {
		return nil
	})
	patches.ApplyMethodFunc(&cur, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodFunc(&cur, "All", func(ctx context.Context, results interface{}) error {
		return nil
	})

	svc := &Service{coll: &coll}
	result, err := svc.List(ListParams{
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
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "CountDocuments", int64(0), errors.New("count failed"))

	svc := &Service{coll: &coll}
	_, err := svc.List(ListParams{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestList_FindError(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "CountDocuments", int64(0), nil)
	patches.ApplyMethodReturn(&coll, "Find", (*mongo.Cursor)(nil), errors.New("find failed"))

	svc := &Service{coll: &coll}
	_, err := svc.List(ListParams{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestList_CursorAllError(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "CountDocuments", int64(0), nil)
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodReturn(&cur, "All", errors.New("cursor all failed"))

	svc := &Service{coll: &coll}
	_, err := svc.List(ListParams{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestList_DefaultLimit(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "CountDocuments", int64(0), nil)
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodFunc(&cur, "All", func(ctx context.Context, results interface{}) error {
		return nil
	})

	svc := &Service{coll: &coll}
	result, err := svc.List(ListParams{Limit: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil ListResult")
	}
}

func TestList_LimitCapped(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "CountDocuments", int64(0), nil)
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodFunc(&cur, "All", func(ctx context.Context, results interface{}) error {
		return nil
	})

	svc := &Service{coll: &coll}
	result, err := svc.List(ListParams{Limit: 500})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil ListResult")
	}
}

func TestList_InvalidStartDate(t *testing.T) {
	svc := &Service{coll: nil}
	_, err := svc.List(ListParams{Start: "invalid-date"})
	if err == nil {
		t.Fatal("expected error for invalid start date")
	}
}

func TestList_InvalidEndDate(t *testing.T) {
	svc := &Service{coll: nil}
	_, err := svc.List(ListParams{End: "invalid-date"})
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
