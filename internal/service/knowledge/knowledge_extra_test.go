package knowledge

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

// --- WithVectorIndex ---

// TestWithVectorIndex_SetsFieldsAndChains verifies WithVectorIndex wires the
// vector repository and embedding function onto the service and returns the
// same service instance for chaining.
func TestWithVectorIndex_SetsFieldsAndChains(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	vecRepo := mockrepo.NewVectorRepository(t)
	embed := func(ctx context.Context, text string) ([]float32, error) {
		return []float32{0.1, 0.2, 0.3}, nil
	}

	svc := NewService(kbRepo)
	returned := svc.WithVectorIndex(vecRepo, embed)

	if returned != svc {
		t.Error("WithVectorIndex should return the same service instance for chaining")
	}
	if svc.vector != repository.VectorRepository(vecRepo) {
		t.Error("vector field should be set to the injected VectorRepository")
	}
	if svc.embed == nil {
		t.Error("embed function should be set")
	}
	if svc.vecCol != "kb_chunks" {
		t.Errorf("vecCol = %q, want kb_chunks", svc.vecCol)
	}
}

// TestWithVectorIndex_NilArgsAllowsTextFallback verifies WithVectorIndex
// accepts nil embed (search falls back to text path).
func TestWithVectorIndex_NilArgsAllowsTextFallback(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("SearchChunks", mock.Anything, "q", 5).Return([]*knowledge.SearchResult{
		{ChunkID: "c1", Content: "hello", Score: 0.9},
	}, nil)

	svc := NewService(kbRepo).WithVectorIndex(nil, nil)
	if svc.vector != nil {
		t.Error("vector should be nil when nil is passed")
	}
	if svc.embed != nil {
		t.Error("embed should be nil when nil is passed")
	}
	// Search should fall back to text since vector is nil.
	results, err := svc.Search("user1", "q", 5, "user")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].ChunkID != "c1" {
		t.Errorf("ChunkID = %q, want c1", results[0].ChunkID)
	}
}

// --- AddChunks ---

// TestAddChunks_WithoutVectorRepo verifies AddChunks stores chunks and updates
// doc status when no vector index is configured.
func TestAddChunks_WithoutVectorRepo(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("AddChunks", mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		chunks := args.Get(1).([]*knowledge.Chunk)
		if len(chunks) != 2 {
			t.Errorf("expected 2 chunks, got %d", len(chunks))
		}
		if chunks[0].DocID != "doc1" {
			t.Errorf("DocID = %q, want doc1", chunks[0].DocID)
		}
	})
	kbRepo.On("UpdateDocStatus", mock.Anything, "doc1", knowledge.StatusIndexing, 2).Return(nil)

	svc := NewService(kbRepo)
	if err := svc.AddChunks("doc1", []string{"chunk one", "chunk two"}); err != nil {
		t.Fatalf("AddChunks: %v", err)
	}
}

// TestAddChunks_WithVectorUpsert verifies AddChunks embeds texts and upserts
// vectors when a vector index is configured.
func TestAddChunks_WithVectorUpsert(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("AddChunks", mock.Anything, mock.Anything).Return(nil)
	kbRepo.On("UpdateDocStatus", mock.Anything, "doc1", knowledge.StatusIndexing, 2).Return(nil)

	vecRepo := mockrepo.NewVectorRepository(t)
	vecRepo.On("Upsert", mock.Anything, "kb_chunks", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		vectors := args.Get(2).([]repository.VectorPoint)
		if len(vectors) != 2 {
			t.Errorf("expected 2 vectors, got %d", len(vectors))
		}
		if vectors[0].Metadata["doc_id"] != "doc1" {
			t.Errorf("doc_id metadata = %v, want doc1", vectors[0].Metadata["doc_id"])
		}
	})

	embed := func(ctx context.Context, text string) ([]float32, error) {
		return []float32{0.1, 0.2}, nil
	}
	svc := NewService(kbRepo).WithVectorIndex(vecRepo, embed)

	if err := svc.AddChunks("doc1", []string{"a", "b"}); err != nil {
		t.Fatalf("AddChunks: %v", err)
	}
}

// TestAddChunks_EmbedErrorSkipsVectors verifies that when embed fails, vectors
// are skipped but chunks are still stored.
func TestAddChunks_EmbedErrorSkipsVectors(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("AddChunks", mock.Anything, mock.Anything).Return(nil)
	kbRepo.On("UpdateDocStatus", mock.Anything, "doc1", knowledge.StatusIndexing, 1).Return(nil)

	// Note: Upsert must NOT be called, so no expectation is registered.
	// mockery will fail the test if Upsert is invoked.
	vecRepo := mockrepo.NewVectorRepository(t)
	embed := func(ctx context.Context, text string) ([]float32, error) {
		return nil, errors.New("embedding service unavailable")
	}
	svc := NewService(kbRepo).WithVectorIndex(vecRepo, embed)

	if err := svc.AddChunks("doc1", []string{"only chunk"}); err != nil {
		t.Fatalf("AddChunks: %v", err)
	}
}

// TestAddChunks_KBAddChunksError verifies AddChunks surfaces the kb.AddChunks
// error and does not call UpdateDocStatus.
func TestAddChunks_KBAddChunksError(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("AddChunks", mock.Anything, mock.Anything).Return(errors.New("mongo insert failed"))
	// UpdateDocStatus must NOT be called.

	svc := NewService(kbRepo)
	err := svc.AddChunks("doc1", []string{"x"})
	if err == nil {
		t.Fatal("expected error from kb.AddChunks")
	}
	if !strings.Contains(err.Error(), "add chunks") {
		t.Errorf("error should wrap 'add chunks', got %v", err)
	}
	if !strings.Contains(err.Error(), "mongo insert failed") {
		t.Errorf("error should contain underlying cause, got %v", err)
	}
}

// TestAddChunks_UpdateDocStatusError verifies AddChunks returns the
// UpdateDocStatus error (the final return).
func TestAddChunks_UpdateDocStatusError(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("AddChunks", mock.Anything, mock.Anything).Return(nil)
	kbRepo.On("UpdateDocStatus", mock.Anything, "doc1", knowledge.StatusIndexing, 1).Return(errors.New("status update failed"))

	svc := NewService(kbRepo)
	err := svc.AddChunks("doc1", []string{"x"})
	if err == nil {
		t.Fatal("expected error from UpdateDocStatus")
	}
	if !strings.Contains(err.Error(), "status update failed") {
		t.Errorf("error should contain underlying cause, got %v", err)
	}
}

// TestAddChunks_EmptyTexts verifies AddChunks with no texts still calls
// AddChunks (with nil slice) and UpdateDocStatus with chunk count 0.
func TestAddChunks_EmptyTexts(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("AddChunks", mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		chunks := args.Get(1).([]*knowledge.Chunk)
		if chunks != nil {
			t.Errorf("expected nil chunks slice, got %v", chunks)
		}
	})
	kbRepo.On("UpdateDocStatus", mock.Anything, "doc1", knowledge.StatusIndexing, 0).Return(nil)

	svc := NewService(kbRepo)
	if err := svc.AddChunks("doc1", []string{}); err != nil {
		t.Fatalf("AddChunks: %v", err)
	}
}

// --- Search ---

// TestSearch_VectorResults verifies Search returns vector search results when
// the vector index is configured and returns hits.
func TestSearch_VectorResults(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	// SearchChunks must NOT be called (no fallback needed).
	vecRepo := mockrepo.NewVectorRepository(t)
	vecRepo.On("Search", mock.Anything, "kb_chunks", mock.Anything, 5, mock.Anything).Return([]repository.VectorSearchHit{
		{ID: "v1", Score: 0.95},
		{ID: "v2", Score: 0.80},
	}, nil)

	embed := func(ctx context.Context, text string) ([]float32, error) {
		return []float32{0.5, 0.5}, nil
	}
	svc := NewService(kbRepo).WithVectorIndex(vecRepo, embed)

	results, err := svc.Search("user1", "query", 5, "user")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].ChunkID != "v1" {
		t.Errorf("top result ChunkID = %q, want v1", results[0].ChunkID)
	}
	// float32(0.95) upcasts to ~0.94999998 as float64, so use a tolerance.
	if results[0].Score < 0.94 || results[0].Score > 0.96 {
		t.Errorf("top result Score = %v, want ~0.95", results[0].Score)
	}
	if results[1].ChunkID != "v2" {
		t.Errorf("second result ChunkID = %q, want v2", results[1].ChunkID)
	}
}

// TestSearch_TextFallback verifies Search falls back to text search when the
// vector index is not configured.
func TestSearch_TextFallback(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("SearchChunks", mock.Anything, "query", 5).Return([]*knowledge.SearchResult{
		{ChunkID: "t1", Content: "text result", Score: 0.7},
		{ChunkID: "t2", Content: "another", Score: 0.4},
	}, nil)

	svc := NewService(kbRepo) // no vector index
	results, err := svc.Search("user1", "query", 5, "user")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].ChunkID != "t1" {
		t.Errorf("top result ChunkID = %q, want t1", results[0].ChunkID)
	}
	if results[0].Content != "text result" {
		t.Errorf("Content = %q, want 'text result'", results[0].Content)
	}
}

// TestSearch_NoResults verifies Search returns an empty (nil) slice when both
// vector and text search yield nothing.
func TestSearch_NoResults(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("SearchChunks", mock.Anything, "query", 5).Return([]*knowledge.SearchResult{}, nil)

	svc := NewService(kbRepo)
	results, err := svc.Search("user1", "query", 5, "user")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("got %d results, want 0", len(results))
	}
	if results != nil {
		t.Errorf("results should be nil for no matches, got %v", results)
	}
}

// TestSearch_VectorSearchErrorFallsBackToText verifies that when the vector
// search returns an error, Search falls back to text search.
func TestSearch_VectorSearchErrorFallsBackToText(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("SearchChunks", mock.Anything, "query", 3).Return([]*knowledge.SearchResult{
		{ChunkID: "fallback1", Content: "fb", Score: 0.5},
	}, nil)

	vecRepo := mockrepo.NewVectorRepository(t)
	vecRepo.On("Search", mock.Anything, "kb_chunks", mock.Anything, 3, mock.Anything).Return(nil, errors.New("qdrant unavailable"))

	embed := func(ctx context.Context, text string) ([]float32, error) {
		return []float32{0.1}, nil
	}
	svc := NewService(kbRepo).WithVectorIndex(vecRepo, embed)

	results, err := svc.Search("user1", "query", 3, "user")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1 (from text fallback)", len(results))
	}
	if results[0].ChunkID != "fallback1" {
		t.Errorf("ChunkID = %q, want fallback1", results[0].ChunkID)
	}
}

// TestSearch_EmbedErrorFallsBackToText verifies that when embedding fails,
// Search falls back to text search.
func TestSearch_EmbedErrorFallsBackToText(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("SearchChunks", mock.Anything, "q", 4).Return([]*knowledge.SearchResult{
		{ChunkID: "txt1", Score: 0.6},
	}, nil)

	vecRepo := mockrepo.NewVectorRepository(t)
	// vector.Search must NOT be called because embed errors first.
	embed := func(ctx context.Context, text string) ([]float32, error) {
		return nil, errors.New("embed model offline")
	}
	svc := NewService(kbRepo).WithVectorIndex(vecRepo, embed)

	results, err := svc.Search("user1", "q", 4, "user")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].ChunkID != "txt1" {
		t.Errorf("ChunkID = %q, want txt1", results[0].ChunkID)
	}
}

// TestSearch_TextSearchErrorReturnsEmpty verifies that when vector search is
// unavailable and text search errors, Search returns an empty result set.
func TestSearch_TextSearchErrorReturnsEmpty(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("SearchChunks", mock.Anything, "q", 5).Return(nil, errors.New("text index down"))

	svc := NewService(kbRepo)
	results, err := svc.Search("user1", "q", 5, "user")
	if err != nil {
		t.Fatalf("Search should not return a hard error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("got %d results, want 0", len(results))
	}
	if results != nil {
		t.Errorf("results should be nil when text search errors, got %v", results)
	}
}

// TestSearch_SortsByScoreDesc verifies Search sorts results by score descending.
func TestSearch_SortsByScoreDesc(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("SearchChunks", mock.Anything, "q", 10).Return([]*knowledge.SearchResult{
		{ChunkID: "low", Score: 0.1},
		{ChunkID: "high", Score: 0.9},
		{ChunkID: "mid", Score: 0.5},
	}, nil)

	svc := NewService(kbRepo)
	results, err := svc.Search("user1", "q", 10, "user")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	if results[0].ChunkID != "high" {
		t.Errorf("top should be 'high', got %q", results[0].ChunkID)
	}
	if results[2].ChunkID != "low" {
		t.Errorf("last should be 'low', got %q", results[2].ChunkID)
	}
}

// --- UploadFile error path ---

// TestUploadFile_CreateDocError verifies UploadFile surfaces the CreateDoc error.
func TestUploadFile_CreateDocError(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("CreateDoc", mock.Anything, mock.Anything).Return(errors.New("gridfs upload failed"))

	svc := NewService(kbRepo)
	id, err := svc.UploadFile("report.pdf", "application/pdf", strings.NewReader("body"))
	if err == nil {
		t.Fatal("expected error from CreateDoc")
	}
	if id != "" {
		t.Errorf("id should be empty on error, got %q", id)
	}
	if !strings.Contains(err.Error(), "gridfs upload failed") {
		t.Errorf("error should contain underlying cause, got %v", err)
	}
}

// TestUploadFile_ReturnsDocID verifies UploadFile returns the created doc's ID.
func TestUploadFile_ReturnsDocID(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("CreateDoc", mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		doc := args.Get(1).(*knowledge.KnowledgeDoc)
		if doc.FileName != "notes.txt" {
			t.Errorf("FileName = %q, want notes.txt", doc.FileName)
		}
		if doc.FileType != "text/plain" {
			t.Errorf("FileType = %q, want text/plain", doc.FileType)
		}
		if !strings.HasPrefix(doc.GridFSFileID, "fs_") {
			t.Errorf("GridFSFileID should start with fs_, got %q", doc.GridFSFileID)
		}
		if doc.Status != knowledge.StatusUploaded {
			t.Errorf("Status = %v, want %v", doc.Status, knowledge.StatusUploaded)
		}
	})

	svc := NewService(kbRepo)
	id, err := svc.UploadFile("notes.txt", "text/plain", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	if !strings.HasPrefix(id, "kbdoc_") {
		t.Errorf("id should start with kbdoc_, got %q", id)
	}
}

// --- CreateDoc field propagation ---

// TestCreateDoc_FieldPropagation verifies CreateDoc populates all fields on the
// created document before persisting.
func TestCreateDoc_FieldPropagation(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("CreateDoc", mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		doc := args.Get(1).(*knowledge.KnowledgeDoc)
		if doc.UserID != "user42" {
			t.Errorf("UserID = %q, want user42", doc.UserID)
		}
		if doc.Title != "My Title" {
			t.Errorf("Title = %q, want My Title", doc.Title)
		}
		if doc.FileName != "data.csv" {
			t.Errorf("FileName = %q, want data.csv", doc.FileName)
		}
		if doc.FileType != "text/csv" {
			t.Errorf("FileType = %q, want text/csv", doc.FileType)
		}
		if doc.SizeBytes != 2048 {
			t.Errorf("SizeBytes = %d, want 2048", doc.SizeBytes)
		}
		if doc.GridFSFileID != "fs_xyz" {
			t.Errorf("GridFSFileID = %q, want fs_xyz", doc.GridFSFileID)
		}
		if doc.Status != knowledge.StatusUploaded {
			t.Errorf("Status = %v, want %v", doc.Status, knowledge.StatusUploaded)
		}
		if !strings.HasPrefix(doc.ID, "kbdoc_") {
			t.Errorf("ID should start with kbdoc_, got %q", doc.ID)
		}
	})

	doc, err := NewService(kbRepo).CreateDoc("user42", "My Title", "data.csv", "text/csv", 2048, "fs_xyz")
	if err != nil {
		t.Fatalf("CreateDoc: %v", err)
	}
	if doc.UserID != "user42" {
		t.Errorf("returned doc UserID = %q", doc.UserID)
	}
	if doc.SizeBytes != 2048 {
		t.Errorf("returned doc SizeBytes = %d", doc.SizeBytes)
	}
}

// --- DeleteDoc cascade ---

// TestDeleteDoc_CascadeDeletesChunks verifies DeleteDoc deletes the doc and
// cascades to delete its chunks.
func TestDeleteDoc_CascadeDeletesChunks(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("DeleteDoc", mock.Anything, "kbdoc_cascade").Return(nil)
	kbRepo.On("DeleteChunks", mock.Anything, "kbdoc_cascade").Return(int64(5), nil)

	svc := NewService(kbRepo)
	if err := svc.DeleteDoc("kbdoc_cascade"); err != nil {
		t.Fatalf("DeleteDoc: %v", err)
	}
}

// TestDeleteDoc_ChunkDeletionErrorStillReturnsNil verifies DeleteDoc ignores
// chunk deletion errors and returns nil (chunks are best-effort cleanup).
func TestDeleteDoc_ChunkDeletionErrorStillReturnsNil(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("DeleteDoc", mock.Anything, "kbdoc_ok").Return(nil)
	kbRepo.On("DeleteChunks", mock.Anything, "kbdoc_ok").Return(int64(0), errors.New("chunks delete failed"))

	// DeleteDoc on the service returns nil because the doc was deleted successfully.
	svc := NewService(kbRepo)
	if err := svc.DeleteDoc("kbdoc_ok"); err != nil {
		t.Fatalf("DeleteDoc should not return error when doc delete succeeds: %v", err)
	}
}

// --- ListDocs error path ---

// TestListDocs_RepoError verifies ListDocs surfaces the repository error.
func TestListDocs_RepoError(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("ListDocs", mock.Anything, "user1", int64(0), int64(100)).Return(nil, int64(0), errors.New("db connection lost"))

	docs, err := NewService(kbRepo).ListDocs("user1")
	if err == nil {
		t.Fatal("expected error from ListDocs")
	}
	if docs != nil {
		t.Errorf("docs should be nil on error, got %v", docs)
	}
	if !strings.Contains(err.Error(), "db connection lost") {
		t.Errorf("error should contain underlying cause, got %v", err)
	}
}

// --- ListAllDocs error path ---

// TestListAllDocs_RepoError verifies ListAllDocs surfaces the repository error.
func TestListAllDocs_RepoError(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("ListAllDocs", mock.Anything).Return(nil, errors.New("aggregate failed"))

	docs, err := NewService(kbRepo).ListAllDocs()
	if err == nil {
		t.Fatal("expected error from ListAllDocs")
	}
	if docs != nil {
		t.Errorf("docs should be nil on error, got %v", docs)
	}
	if !strings.Contains(err.Error(), "aggregate failed") {
		t.Errorf("error should contain underlying cause, got %v", err)
	}
}

// --- GetDoc error path with nil doc ---

// TestGetDoc_NilDocNoError verifies GetDoc returns a nil doc without error
// when the repository returns (nil, nil).
func TestGetDoc_NilDocNoError(t *testing.T) {
	kbRepo := mockrepo.NewKBRepository(t)
	kbRepo.On("GetDoc", mock.Anything, "missing").Return((*knowledge.KnowledgeDoc)(nil), nil)

	doc, err := NewService(kbRepo).GetDoc("missing")
	if err != nil {
		t.Fatalf("GetDoc should not return error: %v", err)
	}
	if doc != nil {
		t.Errorf("doc should be nil, got %v", doc)
	}
}
