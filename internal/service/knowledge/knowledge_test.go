package knowledge

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

func TestGenShortID(t *testing.T) {
	id := genShortID()
	if id == "" {
		t.Error("genShortID should not be empty")
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

// ===========================================
// Mockery-based tests for layered architecture
// ===========================================

func TestNewService_Knowledge(t *testing.T) {
	repo := mockrepo.NewKBRepository(t)
	s := NewService(repo)
	if s == nil {
		t.Fatal("NewService should not return nil")
	}
	if s.kb != repo {
		t.Error("Service.kb should be the injected repository")
	}
}

func TestCreateDoc_Success(t *testing.T) {
	repo := mockrepo.NewKBRepository(t)
	repo.On("CreateDoc", mock.Anything, mock.Anything).Return(nil)

	doc, err := NewService(repo).CreateDoc("user1", "test title", "test.pdf", "application/pdf", 1024, "fs_abc")
	if err != nil {
		t.Fatalf("CreateDoc failed: %v", err)
	}
	if doc == nil || doc.Title != "test title" {
		t.Errorf("unexpected doc: %+v", doc)
	}
}

func TestCreateDoc_InsertError(t *testing.T) {
	repo := mockrepo.NewKBRepository(t)
	repo.On("CreateDoc", mock.Anything, mock.Anything).Return(fmt.Errorf("db down"))

	_, err := NewService(repo).CreateDoc("user1", "test", "test.pdf", "application/pdf", 1024, "fs_abc")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetDoc_Success(t *testing.T) {
	repo := mockrepo.NewKBRepository(t)
	repo.On("GetDoc", mock.Anything, "kbdoc_test1234").Return(&knowledge.KnowledgeDoc{
		ID: "kbdoc_test1234", Title: "My Doc", UserID: "user1", Status: knowledge.StatusIndexing,
	}, nil)

	doc, err := NewService(repo).GetDoc("kbdoc_test1234")
	if err != nil {
		t.Fatalf("GetDoc failed: %v", err)
	}
	if doc.Title != "My Doc" {
		t.Errorf("Title: got %q, want My Doc", doc.Title)
	}
}

func TestGetDoc_NotFound(t *testing.T) {
	repo := mockrepo.NewKBRepository(t)
	repo.On("GetDoc", mock.Anything, "missing").Return((*knowledge.KnowledgeDoc)(nil), fmt.Errorf("not found"))

	_, err := NewService(repo).GetDoc("missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteDoc_Success(t *testing.T) {
	repo := mockrepo.NewKBRepository(t)
	repo.On("DeleteDoc", mock.Anything, "kbdoc_1").Return(nil)
	repo.On("DeleteChunks", mock.Anything, "kbdoc_1").Return(int64(0), nil)

	if err := NewService(repo).DeleteDoc("kbdoc_1"); err != nil {
		t.Fatalf("DeleteDoc failed: %v", err)
	}
}

func TestDeleteDoc_NotFound(t *testing.T) {
	repo := mockrepo.NewKBRepository(t)
	repo.On("DeleteDoc", mock.Anything, "missing").Return(fmt.Errorf("not found"))

	err := NewService(repo).DeleteDoc("missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListDocs_Success(t *testing.T) {
	repo := mockrepo.NewKBRepository(t)
	repo.On("ListDocs", mock.Anything, "user1", int64(0), int64(100)).Return([]*knowledge.KnowledgeDoc{
		{ID: "d1", Title: "Doc 1", UserID: "user1"},
		{ID: "d2", Title: "Doc 2", UserID: "user1"},
	}, int64(2), nil)

	docs, err := NewService(repo).ListDocs("user1")
	if err != nil {
		t.Fatalf("ListDocs failed: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("got %d docs, want 2", len(docs))
	}
}

func TestListDocs_Empty(t *testing.T) {
	repo := mockrepo.NewKBRepository(t)
	repo.On("ListDocs", mock.Anything, "user1", int64(0), int64(100)).Return([]*knowledge.KnowledgeDoc{}, int64(2), nil)

	docs, err := NewService(repo).ListDocs("user1")
	if err != nil {
		t.Fatalf("ListDocs failed: %v", err)
	}
	if len(docs) != 0 {
		t.Fatalf("expected empty, got %d", len(docs))
	}
}

func TestListAllDocs_Success(t *testing.T) {
	repo := mockrepo.NewKBRepository(t)
	repo.On("ListAllDocs", mock.Anything).Return([]*knowledge.KnowledgeDoc{
		{ID: "d1"}, {ID: "d2"}, {ID: "d3"},
	}, nil)

	docs, err := NewService(repo).ListAllDocs()
	if err != nil {
		t.Fatalf("ListAllDocs failed: %v", err)
	}
	if len(docs) != 3 {
		t.Fatalf("got %d docs, want 3", len(docs))
	}
}

func TestUploadFile_Success(t *testing.T) {
	repo := mockrepo.NewKBRepository(t)
	repo.On("CreateDoc", mock.Anything, mock.Anything).Return(nil)

	fileID, err := NewService(repo).UploadFile("test.pdf", "application/pdf", strings.NewReader("content"))
	if err != nil {
		t.Fatalf("UploadFile failed: %v", err)
	}
	if fileID == "" {
		t.Error("expected non-empty fileID")
	}
}
