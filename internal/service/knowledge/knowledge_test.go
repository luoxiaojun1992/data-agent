package knowledge

import (
	"context"
	"errors"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	"go.mongodb.org/mongo-driver/mongo"
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
	s := NewService(db)
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
