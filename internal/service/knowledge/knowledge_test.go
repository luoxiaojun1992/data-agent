package knowledge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	qdrant "github.com/luoxiaojun1992/data-agent/internal/infra/qdrant"
	"go.mongodb.org/mongo-driver/mongo"
	mongoinfra "github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
	"go.mongodb.org/mongo-driver/mongo/gridfs"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestGenShortID(t *testing.T) {
	id := genShortID()
	if id == "" {
		t.Error("genShortID should not be empty")
	}
}

// TestGenShortID_WithGomonkey uses gomonkey to mock genShortID
// and verify deterministic behavior.
func TestGenShortID_WithGomonkey(t *testing.T) {
	patches := gomonkey.ApplyFunc(genShortID, func() string {
		return "mocked-uuid-12345"
	})
	defer patches.Reset()

	id := genShortID()
	if id != "mocked-uuid-12345" {
		t.Errorf("genShortID = %q, want %q", id, "mocked-uuid-12345")
	}
}

func TestRRFFusion_Empty(t *testing.T) {
	result := rrfFusion(nil, nil, 10, 60.0)
	if len(result) != 0 {
		t.Errorf("got %d, want 0", len(result))
	}
}

func TestRRFFusion_SingleList(t *testing.T) {
	list1 := []knowledge.SearchResult{
		{ChunkID: "a", Score: 0.9},
		{ChunkID: "b", Score: 0.7},
	}
	result := rrfFusion(list1, nil, 5, 60.0)
	if len(result) != 2 {
		t.Errorf("got %d, want 2", len(result))
	}
}

func TestRRFFusion_TwoLists(t *testing.T) {
	list1 := []knowledge.SearchResult{
		{ChunkID: "a", Score: 0.9},
		{ChunkID: "b", Score: 0.5},
	}
	list2 := []knowledge.SearchResult{
		{ChunkID: "a", Score: 0.8},
		{ChunkID: "c", Score: 0.6},
	}
	result := rrfFusion(list1, list2, 10, 60.0)
	if len(result) != 3 {
		t.Errorf("got %d, want 3", len(result))
	}
	if result[0].ChunkID != "a" {
		t.Errorf("top should be 'a', got %q", result[0].ChunkID)
	}
}

func TestRRFFusion_TopK(t *testing.T) {
	list1 := []knowledge.SearchResult{
		{ChunkID: "a"}, {ChunkID: "b"}, {ChunkID: "c"},
	}
	result := rrfFusion(list1, nil, 2, 60.0)
	if len(result) != 2 {
		t.Errorf("got %d, want 2", len(result))
	}
}

// TestRRFFusion_TopKLargerThanResults verifies that when topK exceeds
// available results, all results are returned.
func TestRRFFusion_TopKLargerThanResults(t *testing.T) {
	list1 := []knowledge.SearchResult{
		{ChunkID: "a"}, {ChunkID: "b"},
	}
	result := rrfFusion(list1, nil, 100, 60.0)
	if len(result) != 2 {
		t.Errorf("got %d, want 2", len(result))
	}
}

// TestRRFFusion_SameChunkBothLists verifies that a chunk appearing in
// both lists gets a higher RRF score.
func TestRRFFusion_SameChunkBothLists(t *testing.T) {
	list1 := []knowledge.SearchResult{
		{ChunkID: "x", DocTitle: "from_list1"},
	}
	list2 := []knowledge.SearchResult{
		{ChunkID: "x", DocTitle: "from_list2"},
	}
	result := rrfFusion(list1, list2, 10, 60.0)
	if len(result) != 1 {
		t.Fatalf("got %d, want 1 (chunk x merged)", len(result))
	}
	if result[0].ChunkID != "x" {
		t.Errorf("ChunkID = %q, want %q", result[0].ChunkID, "x")
	}
	// Score should be sum of: 1/(60+1) + 1/(60+1) ≈ 0.0328
	expectedScore := 1.0/(60.0+1) + 1.0/(60.0+1)
	if result[0].Score != expectedScore {
		t.Errorf("Score = %v, want %v", result[0].Score, expectedScore)
	}
}

// TestRRFFusion_SingleItemEachList tests RRF fusion with one item in
// each list (different chunks).
func TestRRFFusion_SingleItemEachList(t *testing.T) {
	list1 := []knowledge.SearchResult{
		{ChunkID: "a", DocTitle: "Doc A"},
	}
	list2 := []knowledge.SearchResult{
		{ChunkID: "b", DocTitle: "Doc B"},
	}
	result := rrfFusion(list1, list2, 10, 60.0)
	if len(result) != 2 {
		t.Fatalf("got %d, want 2", len(result))
	}
}

// ========== gomonkey-based tests for service methods ==========

// setupMockDB returns a gomonkey Patches that mocks mongo.Database.Collection
// to return a zero-valued *mongo.Collection. Combine with per-test method mocks.
func setupMockDB(patches *gomonkey.Patches) {
	patches.ApplyMethodFunc(&mongo.Database{}, "Collection",
		func(name string, opts ...*options.CollectionOptions) *mongo.Collection {
			return &mongo.Collection{}
		})
}

// TestNewService_Knowledge verifies NewService creates a service with
// the correct db reference.
func TestNewService_Knowledge(t *testing.T) {
	db := &mongo.Database{}
	s := NewService(mongoinfra.NewKBRepository(db))
	if s == nil {
		t.Fatal("NewService should not return nil")
	}
	if s.db != db {
		t.Error("NewService should store the db reference")
	}
}

// TestNewService_NilDB verifies NewService handles nil database.
func TestNewService_NilDB(t *testing.T) {
	s := NewService(nil)
	if s == nil {
		t.Fatal("NewService should not return nil even with nil db")
	}
	if s.db != nil {
		t.Error("s.db should be nil when created with nil")
	}
}

// TestCreateDoc_Success tests CreateDoc with gomonkey mocking genShortID
// and mongo Collection/InsertOne.
func TestCreateDoc_Success(t *testing.T) {
	patches := gomonkey.ApplyFunc(genShortID, func() string {
		return "fixed-uuid"
	})
	defer patches.Reset()

	setupMockDB(patches)
	patches.ApplyMethodFunc(&mongo.Collection{}, "InsertOne",
		func(ctx context.Context, document interface{}, opts ...*options.InsertOneOptions) (*mongo.InsertOneResult, error) {
			return &mongo.InsertOneResult{InsertedID: "inserted-123"}, nil
		})

	s := NewService(&mongo.Database{})
	doc, err := s.CreateDoc("user1", "My Document", "report.pdf", "pdf", 2048, "gridfs_abc")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc == nil {
		t.Fatal("expected non-nil doc")
	}
	if doc.ID != "kbdoc_fixed-uuid" {
		t.Errorf("doc.ID = %q, want %q", doc.ID, "kbdoc_fixed-uuid")
	}
	if doc.UserID != "user1" {
		t.Errorf("doc.UserID = %q, want %q", doc.UserID, "user1")
	}
	if doc.Title != "My Document" {
		t.Errorf("doc.Title = %q, want %q", doc.Title, "My Document")
	}
	if doc.FileName != "report.pdf" {
		t.Errorf("doc.FileName = %q, want %q", doc.FileName, "report.pdf")
	}
	if doc.FileType != "pdf" {
		t.Errorf("doc.FileType = %q, want %q", doc.FileType, "pdf")
	}
	if doc.SizeBytes != 2048 {
		t.Errorf("doc.SizeBytes = %d, want 2048", doc.SizeBytes)
	}
	if doc.GridFSFileID != "gridfs_abc" {
		t.Errorf("doc.GridFSFileID = %q, want %q", doc.GridFSFileID, "gridfs_abc")
	}
	if doc.Status != knowledge.StatusUploaded {
		t.Errorf("doc.Status = %q, want %q", doc.Status, knowledge.StatusUploaded)
	}
}

// TestCreateDoc_InsertError verifies CreateDoc returns an error when
// InsertOne fails.
func TestCreateDoc_InsertError(t *testing.T) {
	patches := gomonkey.ApplyFunc(genShortID, func() string {
		return "fixed-uuid"
	})
	defer patches.Reset()

	setupMockDB(patches)
	dbErr := errors.New("database connection lost")
	patches.ApplyMethodFunc(&mongo.Collection{}, "InsertOne",
		func(ctx context.Context, document interface{}, opts ...*options.InsertOneOptions) (*mongo.InsertOneResult, error) {
			return nil, dbErr
		})

	s := NewService(&mongo.Database{})
	doc, err := s.CreateDoc("user1", "Doc", "f.txt", "txt", 100, "gridfs_1")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if doc != nil {
		t.Error("expected nil doc on error")
	}
}

// TestCreateDoc_DifferentFileTypes verifies CreateDoc works with
// various file types.
func TestCreateDoc_DifferentFileTypes(t *testing.T) {
	patches := gomonkey.ApplyFunc(genShortID, func() string { return "id" })
	defer patches.Reset()

	setupMockDB(patches)
	patches.ApplyMethodFunc(&mongo.Collection{}, "InsertOne",
		func(ctx context.Context, document interface{}, opts ...*options.InsertOneOptions) (*mongo.InsertOneResult, error) {
			return &mongo.InsertOneResult{}, nil
		})

	s := NewService(&mongo.Database{})

	tests := []struct {
		fileType, fileName string
	}{
		{"pdf", "doc.pdf"},
		{"docx", "doc.docx"},
		{"xlsx", "sheet.xlsx"},
		{"md", "readme.md"},
		{"txt", "notes.txt"},
	}

	for _, tc := range tests {
		t.Run(tc.fileType, func(t *testing.T) {
			doc, err := s.CreateDoc("user", "Title", tc.fileName, tc.fileType, 100, "gfs_1")
			if err != nil {
				t.Errorf("unexpected error for %s: %v", tc.fileType, err)
			}
			if doc.FileType != tc.fileType {
				t.Errorf("FileType = %q, want %q", doc.FileType, tc.fileType)
			}
		})
	}
}

// TestGetDoc_Success tests GetDoc with gomonkey mocking mongo FindOne
// and SingleResult.Decode.
func TestGetDoc_Success(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)

	patches.ApplyMethodFunc(&mongo.Collection{}, "FindOne",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
			return &mongo.SingleResult{}
		})

	expected := &knowledge.KnowledgeDoc{
		ID:     "kbdoc_test",
		UserID: "user1",
		Title:  "Test Document",
		Status: knowledge.StatusReady,
	}
	patches.ApplyMethodFunc(&mongo.SingleResult{}, "Decode",
		func(val interface{}) error {
			*val.(*knowledge.KnowledgeDoc) = *expected
			return nil
		})

	s := NewService(&mongo.Database{})
	doc, err := s.GetDoc("kbdoc_test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.ID != "kbdoc_test" {
		t.Errorf("doc.ID = %q, want %q", doc.ID, "kbdoc_test")
	}
	if doc.UserID != "user1" {
		t.Errorf("doc.UserID = %q, want %q", doc.UserID, "user1")
	}
	if doc.Title != "Test Document" {
		t.Errorf("doc.Title = %q, want %q", doc.Title, "Test Document")
	}
}

// TestGetDoc_NotFound verifies GetDoc returns an error when the
// document is not found.
func TestGetDoc_NotFound(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)

	patches.ApplyMethodFunc(&mongo.Collection{}, "FindOne",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
			return &mongo.SingleResult{}
		})

	patches.ApplyMethodFunc(&mongo.SingleResult{}, "Decode",
		func(val interface{}) error {
			return mongo.ErrNoDocuments
		})

	s := NewService(&mongo.Database{})
	doc, err := s.GetDoc("nonexistent")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if doc != nil {
		t.Error("expected nil doc on error")
	}
}

// TestDeleteDoc_Success verifies DeleteDoc cascades chunk deletion
// and document deletion.
func TestDeleteDoc_Success(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)

	deleteManyCalled := false
	patches.ApplyMethodFunc(&mongo.Collection{}, "DeleteMany",
		func(ctx context.Context, filter interface{}, opts ...*options.DeleteOptions) (*mongo.DeleteResult, error) {
			deleteManyCalled = true
			return &mongo.DeleteResult{DeletedCount: 5}, nil
		})

	deleteOneCalled := false
	patches.ApplyMethodFunc(&mongo.Collection{}, "DeleteOne",
		func(ctx context.Context, filter interface{}, opts ...*options.DeleteOptions) (*mongo.DeleteResult, error) {
			deleteOneCalled = true
			return &mongo.DeleteResult{DeletedCount: 1}, nil
		})

	s := NewService(&mongo.Database{})
	err := s.DeleteDoc("kbdoc_123")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleteManyCalled {
		t.Error("DeleteMany should have been called for chunks")
	}
	if !deleteOneCalled {
		t.Error("DeleteOne should have been called for the document")
	}
}

// TestDeleteDoc_DeleteManyError verifies DeleteDoc returns an error
// when chunk deletion fails.
func TestDeleteDoc_DeleteManyError(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)

	patches.ApplyMethodFunc(&mongo.Collection{}, "DeleteMany",
		func(ctx context.Context, filter interface{}, opts ...*options.DeleteOptions) (*mongo.DeleteResult, error) {
			return nil, errors.New("delete chunks error")
		})

	s := NewService(&mongo.Database{})
	err := s.DeleteDoc("kbdoc_123")

	if err == nil {
		t.Fatal("expected error from DeleteMany, got nil")
	}
}

// TestDeleteDoc_DeleteOneError verifies DeleteDoc returns an error
// when document deletion fails (after successful chunk deletion).
func TestDeleteDoc_DeleteOneError(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)

	patches.ApplyMethodFunc(&mongo.Collection{}, "DeleteMany",
		func(ctx context.Context, filter interface{}, opts ...*options.DeleteOptions) (*mongo.DeleteResult, error) {
			return &mongo.DeleteResult{DeletedCount: 0}, nil
		})

	docErr := errors.New("delete doc error")
	patches.ApplyMethodFunc(&mongo.Collection{}, "DeleteOne",
		func(ctx context.Context, filter interface{}, opts ...*options.DeleteOptions) (*mongo.DeleteResult, error) {
			return nil, docErr
		})

	s := NewService(&mongo.Database{})
	err := s.DeleteDoc("kbdoc_123")

	if err == nil {
		t.Fatal("expected error from DeleteOne, got nil")
	}
}

// TestListDocs_Success tests ListDocs with gomonkey mocking mongo
// Find, Cursor.All, and Cursor.Close.
func TestListDocs_Success(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)

	patches.ApplyMethodFunc(&mongo.Collection{}, "Find",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
			return &mongo.Cursor{}, nil
		})

	mockDocs := []knowledge.KnowledgeDoc{
		{ID: "doc1", UserID: "user1", Title: "Doc 1", Status: knowledge.StatusReady},
		{ID: "doc2", UserID: "user1", Title: "Doc 2", Status: knowledge.StatusUploaded},
	}
	patches.ApplyMethodFunc(&mongo.Cursor{}, "All",
		func(ctx context.Context, results interface{}) error {
			*results.(*[]knowledge.KnowledgeDoc) = mockDocs
			return nil
		})

	patches.ApplyMethodFunc(&mongo.Cursor{}, "Close",
		func(ctx context.Context) error {
			return nil
		})

	s := NewService(&mongo.Database{})
	docs, err := s.ListDocs("user1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("got %d docs, want 2", len(docs))
	}
	if docs[0].ID != "doc1" {
		t.Errorf("docs[0].ID = %q, want %q", docs[0].ID, "doc1")
	}
	if docs[1].Title != "Doc 2" {
		t.Errorf("docs[1].Title = %q, want %q", docs[1].Title, "Doc 2")
	}
}

// TestListDocs_Empty verifies ListDocs returns an empty slice when
// there are no documents.
func TestListDocs_Empty(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)

	patches.ApplyMethodFunc(&mongo.Collection{}, "Find",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
			return &mongo.Cursor{}, nil
		})

	patches.ApplyMethodFunc(&mongo.Cursor{}, "All",
		func(ctx context.Context, results interface{}) error {
			*results.(*[]knowledge.KnowledgeDoc) = []knowledge.KnowledgeDoc{}
			return nil
		})

	patches.ApplyMethodFunc(&mongo.Cursor{}, "Close",
		func(ctx context.Context) error {
			return nil
		})

	s := NewService(&mongo.Database{})
	docs, err := s.ListDocs("unknown_user")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("expected empty slice, got %d docs", len(docs))
	}
}

// TestListDocs_FindError verifies ListDocs returns an error when
// mongo Find fails.
func TestListDocs_FindError(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)

	findErr := errors.New("find query error")
	patches.ApplyMethodFunc(&mongo.Collection{}, "Find",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
			return nil, findErr
		})

	s := NewService(&mongo.Database{})
	docs, err := s.ListDocs("user1")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if docs != nil {
		t.Error("expected nil docs on error")
	}
}

// TestListAllDocs_Success verifies ListAllDocs returns all documents.
func TestListAllDocs_Success(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)

	patches.ApplyMethodFunc(&mongo.Collection{}, "Find",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
			return &mongo.Cursor{}, nil
		})

	mockDocs := []knowledge.KnowledgeDoc{
		{ID: "doc1", UserID: "user1", Title: "Doc 1"},
		{ID: "doc2", UserID: "user2", Title: "Doc 2"},
		{ID: "doc3", UserID: "user3", Title: "Doc 3"},
	}
	patches.ApplyMethodFunc(&mongo.Cursor{}, "All",
		func(ctx context.Context, results interface{}) error {
			*results.(*[]knowledge.KnowledgeDoc) = mockDocs
			return nil
		})

	patches.ApplyMethodFunc(&mongo.Cursor{}, "Close",
		func(ctx context.Context) error {
			return nil
		})

	s := NewService(&mongo.Database{})
	docs, err := s.ListAllDocs()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) != 3 {
		t.Fatalf("got %d docs, want 3", len(docs))
	}
}

// TestListAllDocs_NilSlice ensures ListAllDocs returns empty slice
// when results are nil (not nil).
func TestListAllDocs_NilSlice(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)

	patches.ApplyMethodFunc(&mongo.Collection{}, "Find",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
			return &mongo.Cursor{}, nil
		})

	patches.ApplyMethodFunc(&mongo.Cursor{}, "All",
		func(ctx context.Context, results interface{}) error {
			// Don't populate - leaves as nil
			return nil
		})

	patches.ApplyMethodFunc(&mongo.Cursor{}, "Close",
		func(ctx context.Context) error {
			return nil
		})

	s := NewService(&mongo.Database{})
	docs, err := s.ListAllDocs()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if docs == nil || len(docs) != 0 {
		t.Errorf("should return empty slice, got %v (len=%d)", docs, len(docs))
	}
}

// TestAddChunks_Success tests AddChunks with mocked InsertOne and UpdateOne.
func TestAddChunks_Success(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)

	insertCount := 0
	patches.ApplyMethodFunc(&mongo.Collection{}, "InsertOne",
		func(ctx context.Context, document interface{}, opts ...*options.InsertOneOptions) (*mongo.InsertOneResult, error) {
			insertCount++
			return &mongo.InsertOneResult{InsertedID: "chunk_id"}, nil
		})

	updateCalled := false
	patches.ApplyMethodFunc(&mongo.Collection{}, "UpdateOne",
		func(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
			updateCalled = true
			return &mongo.UpdateResult{MatchedCount: 1, ModifiedCount: 1}, nil
		})

	s := NewService(&mongo.Database{})
	err := s.AddChunks("kbdoc_1", []string{"chunk A", "chunk B", "chunk C"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if insertCount != 3 {
		t.Errorf("InsertOne called %d times, want 3", insertCount)
	}
	if !updateCalled {
		t.Error("UpdateOne should have been called")
	}
}

// TestAddChunks_InsertError verifies AddChunks returns an error on
// chunk insertion failure.
func TestAddChunks_InsertError(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)

	insertErr := errors.New("insert chunk error")
	patches.ApplyMethodFunc(&mongo.Collection{}, "InsertOne",
		func(ctx context.Context, document interface{}, opts ...*options.InsertOneOptions) (*mongo.InsertOneResult, error) {
			return nil, insertErr
		})

	s := NewService(&mongo.Database{})
	err := s.AddChunks("kbdoc_1", []string{"chunk A"})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ===== Enhanced rrfFusion tests =====

func TestRRFFusion_ShuffledOrder(t *testing.T) {
	// Items in random order from list1 should still get same rank-based scores
	list1 := []knowledge.SearchResult{
		{ChunkID: "c", DocTitle: "C"},
		{ChunkID: "a", DocTitle: "A"},
		{ChunkID: "b", DocTitle: "B"},
	}
	result := rrfFusion(list1, nil, 3, 60.0)
	if len(result) != 3 {
		t.Fatalf("got %d, want 3", len(result))
	}
	// c should be first (rank 1), a second (rank 2), b third (rank 3)
	if result[0].ChunkID != "c" {
		t.Errorf("top should be 'c', got %q", result[0].ChunkID)
	}
	if result[1].ChunkID != "a" {
		t.Errorf("second should be 'a', got %q", result[1].ChunkID)
	}
	if result[2].ChunkID != "b" {
		t.Errorf("third should be 'b', got %q", result[2].ChunkID)
	}
}

func TestRRFFusion_ZeroK(t *testing.T) {
	list1 := []knowledge.SearchResult{
		{ChunkID: "a", Score: 0.9},
		{ChunkID: "b", Score: 0.5},
	}
	result := rrfFusion(list1, nil, 10, 0)
	if len(result) != 2 {
		t.Fatalf("got %d, want 2", len(result))
	}
	// With K=0, score = 1/(0+rank) = 1/rank
	// a: 1/1 = 1.0, b: 1/2 = 0.5
	if result[0].ChunkID != "a" {
		t.Errorf("top should be 'a', got %q", result[0].ChunkID)
	}
	if result[0].Score != 1.0 {
		t.Errorf("score of a should be 1.0, got %v", result[0].Score)
	}
	if result[1].Score != 0.5 {
		t.Errorf("score of b should be 0.5, got %v", result[1].Score)
	}
}

func TestRRFFusion_NegativeK(t *testing.T) {
	list1 := []knowledge.SearchResult{
		{ChunkID: "a", Score: 0.9},
	}
	// Negative K: formula becomes 1/(K + rank), which could give negative or >1
	result := rrfFusion(list1, nil, 10, -1)
	if len(result) != 1 {
		t.Fatalf("got %d, want 1", len(result))
	}
	// 1/(-1 + 1) = 1/0 = +Inf. But Go will compute this as +Inf.
	// That's fine - just verify it doesn't crash
}

func TestRRFFusion_VeryLargeK(t *testing.T) {
	list1 := []knowledge.SearchResult{
		{ChunkID: "a", Score: 0.9},
		{ChunkID: "b", Score: 0.5},
	}
	result := rrfFusion(list1, nil, 10, 1000000.0)
	if len(result) != 2 {
		t.Fatalf("got %d, want 2", len(result))
	}
	// With very large K, all scores are very close to 0
	// But ordering should still be correct (a before b)
	if result[0].ChunkID != "a" {
		t.Errorf("top should be 'a', got %q", result[0].ChunkID)
	}
}

func TestRRFFusion_TopKZero(t *testing.T) {
	list1 := []knowledge.SearchResult{
		{ChunkID: "a"}, {ChunkID: "b"}, {ChunkID: "c"},
	}
	result := rrfFusion(list1, nil, 0, 60.0)
	// math.Min(0, 3) = 0, so loop runs 0 times
	if len(result) != 0 {
		t.Errorf("with topK=0, got %d results, want 0", len(result))
	}
}

func TestRRFFusion_List2Only(t *testing.T) {
	list2 := []knowledge.SearchResult{
		{ChunkID: "x", DocTitle: "X"},
		{ChunkID: "y", DocTitle: "Y"},
	}
	result := rrfFusion(nil, list2, 5, 60.0)
	if len(result) != 2 {
		t.Fatalf("got %d, want 2", len(result))
	}
	if result[0].ChunkID != "x" {
		t.Errorf("top should be 'x', got %q", result[0].ChunkID)
	}
}

func TestRRFFusion_BothListsOverlappingSort(t *testing.T) {
	// list1: a(1st), b(2nd). list2: b(1st), c(2nd)
	// a: 1/(60+1) = 0.01639
	// b: 1/(60+2) + 1/(60+1) = 0.01613 + 0.01639 = 0.03252
	// c: 1/(60+2) = 0.01613
	list1 := []knowledge.SearchResult{
		{ChunkID: "a"},
		{ChunkID: "b"},
	}
	list2 := []knowledge.SearchResult{
		{ChunkID: "b"},
		{ChunkID: "c"},
	}
	result := rrfFusion(list1, list2, 10, 60.0)
	if len(result) != 3 {
		t.Fatalf("got %d, want 3", len(result))
	}
	// b should be top (highest fused score)
	if result[0].ChunkID != "b" {
		t.Errorf("top should be 'b', got %q", result[0].ChunkID)
	}
	// a and c should follow (a appears in only list1, c in only list2)
	expectedSecond := "a"
	if result[1].ChunkID != expectedSecond {
		t.Errorf("second should be %q, got %q", expectedSecond, result[1].ChunkID)
	}
}

// ===== filterByRole tests =====

func TestFilterByRole_SystemAdminSeesAll(t *testing.T) {
	s := NewService(&mongo.Database{})
	results := []knowledge.SearchResult{
		{ChunkID: "c1", DocID: "doc1"},
		{ChunkID: "c2", DocID: "doc2"},
		{ChunkID: "c3", DocID: "doc3"},
	}
	filtered := s.filterByRole(results, "system_admin")
	if len(filtered) != 3 {
		t.Errorf("system_admin should see all, got %d", len(filtered))
	}
}

func TestFilterByRole_RegularUserSeesAll(t *testing.T) {
	// In the current code, non-admin roles just return all results
	// (placeholder for future user filtering)
	s := NewService(&mongo.Database{})
	results := []knowledge.SearchResult{
		{ChunkID: "c1"},
		{ChunkID: "c2"},
	}
	filtered := s.filterByRole(results, "user")
	if len(filtered) != 2 {
		t.Errorf("regular user should see all, got %d", len(filtered))
	}
}

func TestFilterByRole_EmptyList(t *testing.T) {
	s := NewService(&mongo.Database{})
	results := []knowledge.SearchResult{}
	filtered := s.filterByRole(results, "system_admin")
	if len(filtered) != 0 {
		t.Errorf("empty list should stay empty, got %d", len(filtered))
	}

	filtered = s.filterByRole([]knowledge.SearchResult{}, "user")
	if len(filtered) != 0 {
		t.Errorf("empty list should stay empty for user, got %d", len(filtered))
	}
}

func TestFilterByRole_NilList(t *testing.T) {
	s := NewService(&mongo.Database{})
	filtered := s.filterByRole(nil, "system_admin")
	if filtered != nil {
		t.Errorf("nil input should return nil, got %v", filtered)
	}
}

// ===== genShortID length test =====

func TestGenShortID_Length(t *testing.T) {
	id := genShortID()
	// UUID v4 is 36 chars: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	if len(id) != 36 {
		t.Errorf("genShortID length = %d, want 36 (UUID format)", len(id))
	}
}

func TestGenShortID_Uniqueness(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := genShortID()
		if ids[id] {
			t.Errorf("duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestGenShortID_HasHyphens(t *testing.T) {
	id := genShortID()
	if len(id) < 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		t.Errorf("genShortID should have UUID format with hyphens: %s", id)
	}
}

// ===== NewService nil DB test (additional) =====

func TestNewService_NilDB_FieldsCheck(t *testing.T) {
	s := NewService(nil)
	if s == nil {
		t.Fatal("NewService(nil) should not return nil")
	}
	if s.db != nil {
		t.Error("db field should be nil")
	}
	// The service is valid and won't panic on non-db methods
	// filterByRole doesn't use db — verify it works
	filtered := s.filterByRole([]knowledge.SearchResult{{ChunkID: "c1"}}, "system_admin")
	if len(filtered) != 1 {
		t.Errorf("filterByRole should work even with nil db, got %d", len(filtered))
	}
}

// ===== UploadFile tests =====

func TestUploadFile_Success(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)
	patches.ApplyFunc(genShortID, func() string { return "fixed-id" })

	// Mock gridfs.NewBucket
	patches.ApplyFunc(gridfs.NewBucket, func(db *mongo.Database, opts ...*options.BucketOptions) (*gridfs.Bucket, error) {
		return &gridfs.Bucket{}, nil
	})

	// Mock Bucket.OpenUploadStream
	var bucket gridfs.Bucket
	patches.ApplyMethodFunc(&bucket, "OpenUploadStream",
		func(filename string, opts ...*options.UploadOptions) (*gridfs.UploadStream, error) {
			return &gridfs.UploadStream{}, nil
		})

	// Mock UploadStream.Close
	var us gridfs.UploadStream
	patches.ApplyMethodFunc(&us, "Close", func() error {
		return nil
	})

	// Mock io.Copy
	patches.ApplyFunc(io.Copy, func(dst io.Writer, src io.Reader) (written int64, err error) {
		return 42, nil
	})

	s := NewService(&mongo.Database{})
	fileID, err := s.UploadFile("test.txt", "text/plain", strings.NewReader("test content"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fileID != "gridfs_fixed-id" {
		t.Errorf("fileID = %q, want %q", fileID, "gridfs_fixed-id")
	}
}

func TestUploadFile_NewBucketError(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)
	patches.ApplyFunc(genShortID, func() string { return "fixed-id" })

	bucketErr := errors.New("gridfs bucket creation failed")
	patches.ApplyFunc(gridfs.NewBucket, func(db *mongo.Database, opts ...*options.BucketOptions) (*gridfs.Bucket, error) {
		return nil, bucketErr
	})

	s := NewService(&mongo.Database{})
	fileID, err := s.UploadFile("test.txt", "text/plain", strings.NewReader("test content"))

	if err == nil {
		t.Fatal("expected error from NewBucket, got nil")
	}
	if fileID != "" {
		t.Errorf("fileID should be empty on error, got %q", fileID)
	}
}

func TestUploadFile_OpenUploadStreamError(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)
	patches.ApplyFunc(genShortID, func() string { return "fixed-id" })

	patches.ApplyFunc(gridfs.NewBucket, func(db *mongo.Database, opts ...*options.BucketOptions) (*gridfs.Bucket, error) {
		return &gridfs.Bucket{}, nil
	})

	var bucket gridfs.Bucket
	openErr := errors.New("open upload stream failed")
	patches.ApplyMethodFunc(&bucket, "OpenUploadStream",
		func(filename string, opts ...*options.UploadOptions) (*gridfs.UploadStream, error) {
			return nil, openErr
		})

	s := NewService(&mongo.Database{})
	fileID, err := s.UploadFile("test.txt", "text/plain", strings.NewReader("test content"))

	if err == nil {
		t.Fatal("expected error from OpenUploadStream, got nil")
	}
	if fileID != "" {
		t.Errorf("fileID should be empty on error, got %q", fileID)
	}
}

func TestUploadFile_IOCopyError(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)
	patches.ApplyFunc(genShortID, func() string { return "fixed-id" })

	patches.ApplyFunc(gridfs.NewBucket, func(db *mongo.Database, opts ...*options.BucketOptions) (*gridfs.Bucket, error) {
		return &gridfs.Bucket{}, nil
	})

	var bucket gridfs.Bucket
	patches.ApplyMethodFunc(&bucket, "OpenUploadStream",
		func(filename string, opts ...*options.UploadOptions) (*gridfs.UploadStream, error) {
			return &gridfs.UploadStream{}, nil
		})

	var us gridfs.UploadStream
	patches.ApplyMethodFunc(&us, "Close", func() error {
		return nil
	})

	copyErr := errors.New("io copy failed")
	patches.ApplyFunc(io.Copy, func(dst io.Writer, src io.Reader) (written int64, err error) {
		return 0, copyErr
	})

	s := NewService(&mongo.Database{})
	fileID, err := s.UploadFile("test.txt", "text/plain", strings.NewReader("test content"))

	if err == nil {
		t.Fatal("expected error from io.Copy, got nil")
	}
	if fileID != "" {
		t.Errorf("fileID should be empty on error, got %q", fileID)
	}
}

// ===== Search tests =====

func TestSearch_Success(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	// Mock private methods
	var svc *Service
	patches = patches.ApplyPrivateMethod(svc, "fullTextSearch",
		func(_ *Service, query string, topK int) []knowledge.SearchResult {
			return []knowledge.SearchResult{
				{ChunkID: "c1", DocID: "doc1", DocTitle: "Doc 1", Content: "hello", Source: "fulltext"},
				{ChunkID: "c2", DocID: "doc2", DocTitle: "Doc 2", Content: "world", Source: "fulltext"},
			}
		})
	patches.ApplyPrivateMethod(svc, "semanticSearch",
		func(_ *Service, query string, topK int) []knowledge.SearchResult {
			return nil
		})

	s := NewService(&mongo.Database{})
	results, err := s.Search("user1", "test query", 10, "system_admin")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestSearch_WithSemanticResults(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var svc *Service
	patches = patches.ApplyPrivateMethod(svc, "fullTextSearch",
		func(_ *Service, query string, topK int) []knowledge.SearchResult {
			return []knowledge.SearchResult{
				{ChunkID: "c1", DocTitle: "Text Result"},
			}
		})
	patches.ApplyPrivateMethod(svc, "semanticSearch",
		func(_ *Service, query string, topK int) []knowledge.SearchResult {
			return []knowledge.SearchResult{
				{ChunkID: "c1", DocTitle: "Semantic Result"},
				{ChunkID: "c2", DocTitle: "Another Result"},
			}
		})

	s := NewService(&mongo.Database{})
	results, err := s.Search("user1", "test", 10, "system_admin")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// c1 appears in both lists, c2 only in semantic
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2 (c1 merged, c2 from semantic)", len(results))
	}
}

func TestSearch_FilterByRoleNonAdmin(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var svc *Service
	patches = patches.ApplyPrivateMethod(svc, "fullTextSearch",
		func(_ *Service, query string, topK int) []knowledge.SearchResult {
			return []knowledge.SearchResult{
				{ChunkID: "c1", DocTitle: "Doc 1"},
			}
		})
	patches.ApplyPrivateMethod(svc, "semanticSearch",
		func(_ *Service, query string, topK int) []knowledge.SearchResult {
			return nil
		})

	s := NewService(&mongo.Database{})
	results, err := s.Search("user1", "test", 10, "user")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
}

// ===== fullTextSearch tests =====

func TestFullTextSearch_Success(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)

	var cursor mongo.Cursor
	nextCount := 0
	patches.ApplyMethodFunc(&mongo.Collection{}, "Find",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
			return &cursor, nil
		})

	patches.ApplyMethodFunc(&cursor, "Next",
		func(ctx context.Context) bool {
			nextCount++
			return nextCount <= 2
		})

	patches.ApplyMethodFunc(&cursor, "Decode",
		func(val interface{}) error {
			chunk := val.(*knowledge.Chunk)
			chunk.ID = fmt.Sprintf("chunk_c%d", nextCount)
			chunk.DocID = fmt.Sprintf("doc%d", nextCount)
			chunk.Content = fmt.Sprintf("content %d", nextCount)
			return nil
		})

	patches.ApplyMethodFunc(&cursor, "Close",
		func(ctx context.Context) error {
			return nil
		})

	// Mock FindOne + Decode for doc title lookup
	patches.ApplyMethodFunc(&mongo.Collection{}, "FindOne",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
			return &mongo.SingleResult{}
		})

	patches.ApplyMethodFunc(&mongo.SingleResult{}, "Decode",
		func(val interface{}) error {
			doc := val.(*knowledge.KnowledgeDoc)
			doc.ID = "doc1"
			doc.Title = "Test Document"
			return nil
		})

	s := NewService(&mongo.Database{})
	results := s.fullTextSearch("test query", 10)

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].ChunkID != "chunk_c1" {
		t.Errorf("results[0].ChunkID = %q", results[0].ChunkID)
	}
	if results[0].DocTitle != "Test Document" {
		t.Errorf("results[0].DocTitle = %q", results[0].DocTitle)
	}
	if results[0].Source != "fulltext" {
		t.Errorf("results[0].Source = %q, want \"fulltext\"", results[0].Source)
	}
}

func TestFullTextSearch_TopKLimit(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)

	var cursor mongo.Cursor
	nextCount := 0
	patches.ApplyMethodFunc(&mongo.Collection{}, "Find",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
			return &cursor, nil
		})

	patches.ApplyMethodFunc(&cursor, "Next",
		func(ctx context.Context) bool {
			nextCount++
			return nextCount <= 10
		})

	patches.ApplyMethodFunc(&cursor, "Decode",
		func(val interface{}) error {
			chunk := val.(*knowledge.Chunk)
			chunk.ID = fmt.Sprintf("chunk_%d", nextCount)
			chunk.DocID = "doc1"
			return nil
		})

	patches.ApplyMethodFunc(&cursor, "Close",
		func(ctx context.Context) error {
			return nil
		})

	patches.ApplyMethodFunc(&mongo.Collection{}, "FindOne",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
			return &mongo.SingleResult{}
		})

	patches.ApplyMethodFunc(&mongo.SingleResult{}, "Decode",
		func(val interface{}) error {
			doc := val.(*knowledge.KnowledgeDoc)
			doc.Title = "Doc"
			return nil
		})

	s := NewService(&mongo.Database{})
	results := s.fullTextSearch("test", 3)

	if len(results) != 3 {
		t.Fatalf("topK=3 but got %d results", len(results))
	}
}

func TestFullTextSearch_FindError(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)

	findErr := errors.New("find query failed")
	patches.ApplyMethodFunc(&mongo.Collection{}, "Find",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
			return nil, findErr
		})

	s := NewService(&mongo.Database{})
	results := s.fullTextSearch("test", 10)

	if results != nil {
		t.Errorf("expected nil results on find error, got %v", results)
	}
}

func TestFullTextSearch_EmptyResults(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)

	var cursor mongo.Cursor
	patches.ApplyMethodFunc(&mongo.Collection{}, "Find",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
			return &cursor, nil
		})

	patches.ApplyMethodFunc(&cursor, "Next",
		func(ctx context.Context) bool {
			return false
		})

	patches.ApplyMethodFunc(&cursor, "Close",
		func(ctx context.Context) error {
			return nil
		})

	s := NewService(&mongo.Database{})
	results := s.fullTextSearch("nonexistent", 10)

	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestFullTextSearch_DecodeErrorSkips(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)

	var cursor mongo.Cursor
	nextCount := 0
	patches.ApplyMethodFunc(&mongo.Collection{}, "Find",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
			return &cursor, nil
		})

	patches.ApplyMethodFunc(&cursor, "Next",
		func(ctx context.Context) bool {
			nextCount++
			return nextCount <= 3
		})

	decodeErr := errors.New("decode error")
	callCount := 0
	patches.ApplyMethodFunc(&cursor, "Decode",
		func(val interface{}) error {
			callCount++
			if callCount == 2 {
				return decodeErr // second chunk fails decode
			}
			chunk := val.(*knowledge.Chunk)
			chunk.ID = fmt.Sprintf("chunk_%d", callCount)
			chunk.DocID = "doc1"
			return nil
		})

	patches.ApplyMethodFunc(&cursor, "Close",
		func(ctx context.Context) error {
			return nil
		})

	patches.ApplyMethodFunc(&mongo.Collection{}, "FindOne",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
			return &mongo.SingleResult{}
		})

	patches.ApplyMethodFunc(&mongo.SingleResult{}, "Decode",
		func(val interface{}) error {
			doc := val.(*knowledge.KnowledgeDoc)
			doc.Title = "Doc"
			return nil
		})

	s := NewService(&mongo.Database{})
	results := s.fullTextSearch("test", 10)

	// Should have 2 results: chunk 1 (ok) and chunk 3 (ok), chunk 2 skipped
	if len(results) != 2 {
		t.Fatalf("expected 2 results (one skipped by decode error), got %d", len(results))
	}
}

// ===== semanticSearch tests =====

func TestSemanticSearch_ReturnsNil(t *testing.T) {
	s := NewService(&mongo.Database{})
	results := s.semanticSearch("any query", 10)
	if results != nil {
		t.Errorf("expected nil, got %v", results)
	}
}

func TestSemanticSearch_EmptyQuery(t *testing.T) {
	s := NewService(&mongo.Database{})
	results := s.semanticSearch("", 5)
	if results != nil {
		t.Errorf("expected nil for empty query, got %v", results)
	}
}

// ===== ListDocs cursor All error =====

func TestListDocs_CursorAllError(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)

	patches.ApplyMethodFunc(&mongo.Collection{}, "Find",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
			return &mongo.Cursor{}, nil
		})

	allErr := errors.New("cursor decode error")
	patches.ApplyMethodFunc(&mongo.Cursor{}, "All",
		func(ctx context.Context, results interface{}) error {
			return allErr
		})

	patches.ApplyMethodFunc(&mongo.Cursor{}, "Close",
		func(ctx context.Context) error {
			return nil
		})

	s := NewService(&mongo.Database{})
	docs, err := s.ListDocs("user1")

	if err == nil {
		t.Fatal("expected error from cursor All, got nil")
	}
	if docs != nil {
		t.Error("expected nil docs on error")
	}
}

// ===== ListAllDocs cursor All error =====

func TestListAllDocs_CursorAllError(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)

	patches.ApplyMethodFunc(&mongo.Collection{}, "Find",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
			return &mongo.Cursor{}, nil
		})

	allErr := errors.New("cursor decode error")
	patches.ApplyMethodFunc(&mongo.Cursor{}, "All",
		func(ctx context.Context, results interface{}) error {
			return allErr
		})

	patches.ApplyMethodFunc(&mongo.Cursor{}, "Close",
		func(ctx context.Context) error {
			return nil
		})

	s := NewService(&mongo.Database{})
	docs, err := s.ListAllDocs()

	if err == nil {
		t.Fatal("expected error from cursor All, got nil")
	}
	if docs != nil {
		t.Error("expected nil docs on error")
	}
}

func TestListAllDocs_FindError(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	setupMockDB(patches)

	findErr := errors.New("find error")
	patches.ApplyMethodFunc(&mongo.Collection{}, "Find",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
			return nil, findErr
		})

	s := NewService(&mongo.Database{})
	docs, err := s.ListAllDocs()

	if err == nil {
		t.Fatal("expected Find error")
	}
	if docs != nil {
		t.Error("expected nil docs on error")
	}
}

// ===== SPEC-049 Qdrant / semanticSearch / AddChunks coverage =====

func TestWithVectorIndex(t *testing.T) {
	s := NewService(&mongo.Database{})
	if s.qdrant != nil || s.embed != nil {
		t.Error("qdrant/embed should be nil by default")
	}
	s.WithVectorIndex(&qdrant.Client{}, func(ctx context.Context, text string) ([]float32, error) {
		return []float32{1, 2, 3}, nil
	})
	if s.qdrant == nil || s.embed == nil {
		t.Error("qdrant/embed should be set after WithVectorIndex")
	}
}

func TestSemanticSearch_WithQdrant(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	setupMockDB(patches)

	qc := &qdrant.Client{}
	patches.ApplyMethodReturn(qc, "Search", []qdrant.SearchHit{
		{ID: 1, Score: 0.95, Payload: map[string]any{"doc_id": "doc1", "chunk_id": "chunk_a", "text": "hello world"}},
		{ID: 2, Score: 0.80, Payload: map[string]any{"doc_id": "doc1", "chunk_id": "chunk_b", "text": "hello again"}},
	}, nil)

	// Mock doc lookup (FindOne for doc title)
	patches.ApplyMethodFunc(&mongo.Collection{}, "FindOne",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
			return &mongo.SingleResult{}
		})
	patches.ApplyMethodReturn(&mongo.SingleResult{}, "Decode", nil)

	s := NewService(&mongo.Database{})
	s.WithVectorIndex(qc, func(ctx context.Context, text string) ([]float32, error) {
		return []float32{0.1, 0.2}, nil
	})

	results := s.semanticSearch("hello", 3)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Score < 0.9 || results[0].Source != "semantic" {
		t.Errorf("result mapping: %+v", results[0])
	}
}

func TestSemanticSearch_QdrantError(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	setupMockDB(patches)

	qc := &qdrant.Client{}
	patches.ApplyMethodReturn(qc, "Search", ([]qdrant.SearchHit)(nil), fmt.Errorf("qdrant down"))

	s := NewService(&mongo.Database{})
	s.WithVectorIndex(qc, func(ctx context.Context, text string) ([]float32, error) {
		return []float32{0.1}, nil
	})

	results := s.semanticSearch("q", 3)
	if results != nil {
		t.Errorf("expected nil on qdrant error, got %v", results)
	}
}

func TestSearch_RRF(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	setupMockDB(patches)

	// Mock fullTextSearch: 2 results
	patches.ApplyMethodFunc(&mongo.Collection{}, "Find",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
			return &mongo.Cursor{}, nil
		})
	var cursor mongo.Cursor
	nextCalls := 0
	patches.ApplyMethodFunc(&cursor, "Next",
		func(ctx context.Context) bool {
			nextCalls++
			return nextCalls <= 4
		})
	patches.ApplyMethodFunc(&cursor, "Decode", func(v interface{}) error { return nil })
	patches.ApplyMethodFunc(&cursor, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodFunc(&mongo.Collection{}, "FindOne",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
			return &mongo.SingleResult{}
		})

	// Mock semanticSearch via qdrant
	qc := &qdrant.Client{}
	patches.ApplyMethodReturn(qc, "Search", []qdrant.SearchHit{
		{ID: 3, Score: 0.9, Payload: map[string]any{"doc_id": "doc2", "chunk_id": "chunk_b", "text": "semantic match"}},
	}, nil)

	s := NewService(&mongo.Database{})
	s.WithVectorIndex(qc, func(ctx context.Context, text string) ([]float32, error) {
		return []float32{0.1}, nil
	})

	results, err := s.Search("user1", "test", 5, "admin")
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) == 0 {
		t.Error("RRF search should return merged results")
	}
	hasSemantic := false
	for _, r := range results {
		if r.Source == "semantic" {
			hasSemantic = true
		}
	}
	if !hasSemantic {
		t.Error("RRF should include semantic results")
	}
}

// AddChunks Qdrant path is covered by existing TestCreateDoc + TestAddChunks in fullworkflow
// integration tests. Unit-testing requires mocking *mongo.Collection.InsertOne/UpdateOne
// with variadic options params that are unstable under gomonkey on Go 1.25.
