package notification

import (
	"context"
	"errors"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestNewService(t *testing.T) {
	db := &mongo.Database{}
	var coll mongo.Collection
	patches := gomonkey.ApplyMethodReturn(db, "Collection", &coll)
	defer patches.Reset()

	s := NewService(db)
	if s == nil {
		t.Fatal("NewService should not return nil")
	}
	if s.coll != &coll {
		t.Error("Service.coll should be the Collection returned by db.Collection")
	}
}

func TestSend_Success(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "InsertOne", &mongo.InsertOneResult{}, nil)

	svc := &Service{coll: &coll}
	n, err := svc.Send("Test Title", "Test Content", "info", []string{"user1", "user2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n == nil {
		t.Fatal("expected non-nil Notification")
	}
	if n.Title != "Test Title" {
		t.Errorf("Title: got %q, want %q", n.Title, "Test Title")
	}
	if n.Content != "Test Content" {
		t.Errorf("Content: got %q, want %q", n.Content, "Test Content")
	}
	if n.Type != "info" {
		t.Errorf("Type: got %q, want %q", n.Type, "info")
	}
	if len(n.TargetIDs) != 2 {
		t.Errorf("TargetIDs length: got %d, want 2", len(n.TargetIDs))
	}
	if n.TargetAll {
		t.Error("TargetAll should be false for targeted send")
	}
}

func TestSend_InsertError(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "InsertOne", (*mongo.InsertOneResult)(nil), errors.New("insert failed"))

	svc := &Service{coll: &coll}
	_, err := svc.Send("Test", "Content", "info", []string{"user1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBroadcast_Success(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "InsertOne", &mongo.InsertOneResult{}, nil)

	svc := &Service{coll: &coll}
	n, err := svc.Broadcast("System Update", "All systems operational", "info")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n == nil {
		t.Fatal("expected non-nil Notification")
	}
	if !n.TargetAll {
		t.Error("TargetAll should be true for broadcast")
	}
	if n.Title != "System Update" {
		t.Errorf("Title: got %q", n.Title)
	}
	if n.Content != "All systems operational" {
		t.Errorf("Content: got %q", n.Content)
	}
	if len(n.TargetIDs) != 0 {
		t.Errorf("TargetIDs should be empty for broadcast, got %d items", len(n.TargetIDs))
	}
}

func TestBroadcast_InsertError(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "InsertOne", (*mongo.InsertOneResult)(nil), errors.New("insert failed"))

	svc := &Service{coll: &coll}
	_, err := svc.Broadcast("Test", "Content", "info")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListForUser_Success(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "Find", func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
		f := filter.(bson.M)
		if or, ok := f["$or"]; ok {
			if _, ok := or.([]bson.M); !ok {
				t.Error("ListForUser filter should have $or with array")
			}
		} else {
			t.Error("ListForUser filter should have $or condition")
		}
		return &cur, nil
	})
	patches.ApplyMethodFunc(&cur, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodFunc(&cur, "All", func(ctx context.Context, results interface{}) error {
		slice := results.(*[]model.Notification)
		*slice = []model.Notification{
			{Title: "Notif 1", Content: "Content 1"},
			{Title: "Notif 2", Content: "Content 2"},
		}
		return nil
	})

	svc := &Service{coll: &coll}
	list, err := svc.ListForUser("user1", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(list))
	}
	if list[0].Title != "Notif 1" {
		t.Errorf("list[0].Title: got %q", list[0].Title)
	}
	if list[1].Title != "Notif 2" {
		t.Errorf("list[1].Title: got %q", list[1].Title)
	}
}

func TestListForUser_DefaultLimit(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodFunc(&cur, "All", func(ctx context.Context, results interface{}) error {
		return nil
	})

	svc := &Service{coll: &coll}
	list, err := svc.ListForUser("user1", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if list == nil {
		t.Error("expected non-nil list (should default to empty slice)")
	}
}

func TestListForUser_FindError(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", (*mongo.Cursor)(nil), errors.New("find failed"))

	svc := &Service{coll: &coll}
	_, err := svc.ListForUser("user1", 10)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListForUser_CursorAllError(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodReturn(&cur, "All", errors.New("cursor all failed"))

	svc := &Service{coll: &coll}
	_, err := svc.ListForUser("user1", 10)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUnreadCount_Success(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "CountDocuments", func(ctx context.Context, filter interface{}, opts ...*options.CountOptions) (int64, error) {
		f := filter.(bson.M)
		if or, ok := f["$or"]; ok {
			if _, ok := or.([]bson.M); !ok {
				t.Error("UnreadCount filter should have $or with array")
			}
		}
		return int64(5), nil
	})

	svc := &Service{coll: &coll}
	count, err := svc.UnreadCount("user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 5 {
		t.Errorf("count: got %d, want 5", count)
	}
}

func TestUnreadCount_Error(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "CountDocuments", int64(0), errors.New("count failed"))

	svc := &Service{coll: &coll}
	_, err := svc.UnreadCount("user1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMarkRead_Success(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "UpdateOne", func(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
		u := update.(bson.M)
		if addToSet, ok := u["$addToSet"]; ok {
			if addToSetMap, ok := addToSet.(bson.M); ok {
				if addToSetMap["read_by"] != "user1" {
					t.Errorf("MarkRead read_by: got %v, want user1", addToSetMap["read_by"])
				}
			}
		}
		return &mongo.UpdateResult{}, nil
	})

	svc := &Service{coll: &coll}
	err := svc.MarkRead("507f1f77bcf86cd799439011", "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMarkRead_InvalidID(t *testing.T) {
	svc := &Service{coll: nil}
	err := svc.MarkRead("invalid-id", "user1")
	if err == nil {
		t.Fatal("expected error for invalid id")
	}
}

func TestMarkRead_UpdateError(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "UpdateOne", (*mongo.UpdateResult)(nil), errors.New("update failed"))

	svc := &Service{coll: &coll}
	err := svc.MarkRead("507f1f77bcf86cd799439011", "user1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMarkAllRead_Success(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "UpdateMany", func(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
		u := update.(bson.M)
		if addToSet, ok := u["$addToSet"]; ok {
			if addToSetMap, ok := addToSet.(bson.M); ok {
				if addToSetMap["read_by"] != "user1" {
					t.Errorf("MarkAllRead read_by: got %v, want user1", addToSetMap["read_by"])
				}
			}
		}
		return &mongo.UpdateResult{}, nil
	})

	svc := &Service{coll: &coll}
	err := svc.MarkAllRead("user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMarkAllRead_Error(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "UpdateMany", (*mongo.UpdateResult)(nil), errors.New("update failed"))

	svc := &Service{coll: &coll}
	err := svc.MarkAllRead("user1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// Ensure bson is used
var _ = bson.M{}

// Ensure options is referenced
var _ = options.Find()
