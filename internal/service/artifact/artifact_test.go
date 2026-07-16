package artifact

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/luoxiaojun1992/data-agent/internal/domain/artifact"
	"github.com/luoxiaojun1992/data-agent/internal/infra/seaweedfs"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestNewStorage(t *testing.T) {
	var db mongo.Database
	var coll mongo.Collection
	patches := gomonkey.ApplyMethodReturn(&db, "Collection", &coll)
	defer patches.Reset()

	s := NewStorage(nil, &db)
	if s == nil {
		t.Fatal("NewStorage should not return nil")
	}
}

func TestUpload_Success(t *testing.T) {
	var sw seaweedfs.Client
	var coll mongo.Collection
	var db mongo.Database

	patches := gomonkey.NewPatches()
	defer patches.Reset()

	// Mock seaweedfs Client.Upload to return size + no error
	patches.ApplyMethodReturn(&sw, "Upload", int64(12), nil)
	// Mock mongo Collection.InsertOne
	patches.ApplyMethodReturn(&coll, "InsertOne", &mongo.InsertOneResult{}, nil)
	// Mock Database.Collection
	patches.ApplyMethodReturn(&db, "Collection", &coll)

	s := NewStorage(&sw, &db)
	reader := strings.NewReader("test content")
	art, err := s.Upload("user1", "session1", "task1", "test.txt", "text/plain", reader, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if art == nil {
		t.Fatal("expected non-nil Artifact")
	}
	if art.Name != "test.txt" {
		t.Errorf("Name: got %q, want %q", art.Name, "test.txt")
	}
	if art.SizeBytes != 12 {
		t.Errorf("SizeBytes: got %d, want 12", art.SizeBytes)
	}
}

func TestUpload_SeaweedfsError(t *testing.T) {
	var sw seaweedfs.Client
	var coll mongo.Collection
	var db mongo.Database

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&sw, "Upload", int64(0), errors.New("seaweedfs upload: connection refused"))
	patches.ApplyMethodReturn(&db, "Collection", &coll)

	s := NewStorage(&sw, &db)
	reader := strings.NewReader("test content")
	_, err := s.Upload("user1", "session1", "task1", "test.txt", "text/plain", reader, true)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpload_MongoInsertError(t *testing.T) {
	var sw seaweedfs.Client
	var coll mongo.Collection
	var db mongo.Database

	patches := gomonkey.NewPatches()
	defer patches.Reset()

	// seaweedfs upload succeeds
	patches.ApplyMethodReturn(&sw, "Upload", int64(12), nil)
	// mongo insert fails
	patches.ApplyMethodReturn(&coll, "InsertOne", (*mongo.InsertOneResult)(nil), errors.New("db error"))
	// seaweedfs delete is called as cleanup - mock it
	patches.ApplyMethodReturn(&sw, "Delete", nil)
	// Database.Collection
	patches.ApplyMethodReturn(&db, "Collection", &coll)

	s := NewStorage(&sw, &db)
	reader := strings.NewReader("test content")
	_, err := s.Upload("user1", "session1", "task1", "test.txt", "text/plain", reader, true)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFindByID_Success(t *testing.T) {
	var coll mongo.Collection
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodFunc(&sr, "Decode", func(v interface{}) error {
		art := v.(*artifact.Artifact)
		art.ID = "artifact_test123"
		art.Name = "test.txt"
		art.StoragePath = "artifacts/user1/session1/test.txt"
		return nil
	})

	s := &Storage{coll: &coll}
	art, err := s.FindByID("artifact_test123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if art.ID != "artifact_test123" {
		t.Errorf("ID: got %q", art.ID)
	}
}

func TestFindByID_NotFound(t *testing.T) {
	var coll mongo.Collection
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodReturn(&sr, "Decode", mongo.ErrNoDocuments)

	s := &Storage{coll: &coll}
	_, err := s.FindByID("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFindByID_OtherError(t *testing.T) {
	var coll mongo.Collection
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodReturn(&sr, "Decode", errors.New("db error"))

	s := &Storage{coll: &coll}
	_, err := s.FindByID("artifact_test123")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDelete_NotFound_Idempotent(t *testing.T) {
	var coll mongo.Collection
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodReturn(&sr, "Decode", mongo.ErrNoDocuments)

	s := &Storage{coll: &coll}
	err := s.Delete("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error (should be idempotent): %v", err)
	}
}

func TestDelete_Success(t *testing.T) {
	var sw seaweedfs.Client
	var coll mongo.Collection
	var db mongo.Database
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()

	// Database.Collection
	patches.ApplyMethodReturn(&db, "Collection", &coll)

	// FindByID: FindOne + Decode
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodFunc(&sr, "Decode", func(v interface{}) error {
		art := v.(*artifact.Artifact)
		art.ID = "artifact_test123"
		art.StoragePath = "artifacts/user1/session1/test.txt"
		return nil
	})

	// Delete from seaweedfs succeeds
	patches.ApplyMethodReturn(&sw, "Delete", nil)
	// DeleteOne from mongo succeeds
	patches.ApplyMethodReturn(&coll, "DeleteOne", &mongo.DeleteResult{}, nil)

	s := NewStorage(&sw, &db)
	err := s.Delete("artifact_test123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDelete_SeaweedfsError(t *testing.T) {
	var sw seaweedfs.Client
	var coll mongo.Collection
	var db mongo.Database
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()

	// Database.Collection
	patches.ApplyMethodReturn(&db, "Collection", &coll)

	// FindByID succeeds
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodFunc(&sr, "Decode", func(v interface{}) error {
		art := v.(*artifact.Artifact)
		art.ID = "artifact_test123"
		art.StoragePath = "artifacts/user1/session1/test.txt"
		return nil
	})

	// seaweedfs delete fails
	patches.ApplyMethodReturn(&sw, "Delete", errors.New("seaweedfs delete: connection refused"))

	s := NewStorage(&sw, &db)
	err := s.Delete("artifact_test123")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDelete_MongoError(t *testing.T) {
	var sw seaweedfs.Client
	var coll mongo.Collection
	var db mongo.Database
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()

	// Database.Collection
	patches.ApplyMethodReturn(&db, "Collection", &coll)

	// FindByID succeeds
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodFunc(&sr, "Decode", func(v interface{}) error {
		art := v.(*artifact.Artifact)
		art.ID = "artifact_test123"
		art.StoragePath = "artifacts/user1/session1/test.txt"
		return nil
	})

	// seaweedfs delete succeeds
	patches.ApplyMethodReturn(&sw, "Delete", nil)
	// mongo delete fails
	patches.ApplyMethodReturn(&coll, "DeleteOne", (*mongo.DeleteResult)(nil), errors.New("delete metadata failed"))

	s := NewStorage(&sw, &db)
	err := s.Delete("artifact_test123")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListBySession_Success(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodFunc(&cur, "All", func(ctx context.Context, results interface{}) error {
		slice := results.(*[]artifact.Artifact)
		*slice = []artifact.Artifact{
			{ID: "art1", Name: "file1.txt"},
			{ID: "art2", Name: "file2.txt"},
		}
		return nil
	})

	s := &Storage{coll: &coll}
	arts, err := s.ListBySession("session1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(arts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(arts))
	}
}

func TestListBySession_Error(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", (*mongo.Cursor)(nil), errors.New("find failed"))

	s := &Storage{coll: &coll}
	_, err := s.ListBySession("session1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListByTask_Success(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodFunc(&cur, "All", func(ctx context.Context, results interface{}) error {
		return nil
	})

	s := &Storage{coll: &coll}
	arts, err := s.ListByTask("task1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(arts) != 0 {
		t.Errorf("expected 0 artifacts, got %d", len(arts))
	}
}

func TestListByTask_Error(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", (*mongo.Cursor)(nil), errors.New("find failed"))

	s := &Storage{coll: &coll}
	_, err := s.ListByTask("task1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// Ensure imports are used
var _ = bson.M{}
var _ = io.ReadAll

// ===== Download tests =====

func TestDownload_Success(t *testing.T) {
	var sw seaweedfs.Client
	var coll mongo.Collection
	var db mongo.Database
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()

	patches.ApplyMethodReturn(&db, "Collection", &coll)

	// Mock FindByID (FindOne + Decode)
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodFunc(&sr, "Decode", func(v interface{}) error {
		art := v.(*artifact.Artifact)
		art.ID = "artifact_test123"
		art.Name = "test.txt"
		art.StoragePath = "artifacts/user1/session1/test.txt"
		return nil
	})

	// Mock seaweedfs Download
	patches.ApplyMethodReturn(&sw, "Download", []byte("file content"), nil)

	s := NewStorage(&sw, &db)
	data, art, err := s.Download("artifact_test123")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "file content" {
		t.Errorf("data = %q, want %q", string(data), "file content")
	}
	if art.ID != "artifact_test123" {
		t.Errorf("art.ID = %q", art.ID)
	}
	if art.StoragePath != "artifacts/user1/session1/test.txt" {
		t.Errorf("art.StoragePath = %q", art.StoragePath)
	}
}

func TestDownload_FindByIDError(t *testing.T) {
	var sw seaweedfs.Client
	var coll mongo.Collection
	var db mongo.Database
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()

	patches.ApplyMethodReturn(&db, "Collection", &coll)
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodReturn(&sr, "Decode", mongo.ErrNoDocuments)

	s := NewStorage(&sw, &db)
	data, art, err := s.Download("nonexistent")

	if err == nil {
		t.Fatal("expected error from FindByID, got nil")
	}
	if data != nil {
		t.Error("data should be nil on error")
	}
	if art != nil {
		t.Error("art should be nil on error")
	}
}

func TestDownload_SeaweedfsError(t *testing.T) {
	var sw seaweedfs.Client
	var coll mongo.Collection
	var db mongo.Database
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()

	patches.ApplyMethodReturn(&db, "Collection", &coll)

	// FindByID succeeds
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodFunc(&sr, "Decode", func(v interface{}) error {
		art := v.(*artifact.Artifact)
		art.ID = "artifact_test123"
		art.StoragePath = "artifacts/user1/session1/test.txt"
		return nil
	})

	// seaweedfs Download fails
	patches.ApplyMethodReturn(&sw, "Download", []byte(nil), errors.New("seaweedfs download: connection refused"))

	s := NewStorage(&sw, &db)
	data, art, err := s.Download("artifact_test123")

	if err == nil {
		t.Fatal("expected error from seaweedfs download, got nil")
	}
	if data != nil {
		t.Error("data should be nil on error")
	}
	if art != nil {
		t.Error("art should be nil on error")
	}
}

// ===== ListBySession cursor All error =====

func TestListBySession_CursorAllError(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodReturn(&cur, "Close", nil)
	patches.ApplyMethodFunc(&cur, "All", func(ctx context.Context, results interface{}) error {
		return errors.New("cursor decode error")
	})

	s := &Storage{coll: &coll}
	_, err := s.ListBySession("session1")
	if err == nil {
		t.Fatal("expected error from cursor All, got nil")
	}
}

// ===== ListByTask cursor All error =====

func TestListByTask_CursorAllError(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodReturn(&cur, "Close", nil)
	patches.ApplyMethodFunc(&cur, "All", func(ctx context.Context, results interface{}) error {
		return errors.New("cursor decode error")
	})

	s := &Storage{coll: &coll}
	_, err := s.ListByTask("task1")
	if err == nil {
		t.Fatal("expected error from cursor All, got nil")
	}
}
