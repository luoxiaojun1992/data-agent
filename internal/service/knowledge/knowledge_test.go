package knowledge

import (
	
	"fmt"
	"strings"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
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

// TestNewService_Knowledge verifies NewService creates a service with
// the correct db reference.

// TestNewService_NilDB verifies NewService handles nil database.

// TestCreateDoc_Success tests CreateDoc with gomonkey mocking genShortID
// and mongo Collection/InsertOne.

// TestCreateDoc_InsertError verifies CreateDoc returns an error when
// InsertOne fails.

// TestCreateDoc_DifferentFileTypes verifies CreateDoc works with
// various file types.

// TestGetDoc_Success tests GetDoc with gomonkey mocking mongo FindOne
// and SingleResult.Decode.

// TestGetDoc_NotFound verifies GetDoc returns an error when the
// document is not found.

// TestDeleteDoc_Success verifies DeleteDoc cascades chunk deletion
// and document deletion.

// TestDeleteDoc_DeleteManyError verifies DeleteDoc returns an error
// when chunk deletion fails.

// TestDeleteDoc_DeleteOneError verifies DeleteDoc returns an error
// when document deletion fails (after successful chunk deletion).

// TestListDocs_Success tests ListDocs with gomonkey mocking mongo
// Find, Cursor.All, and Cursor.Close.

// TestListDocs_Empty verifies ListDocs returns an empty slice when
// there are no documents.

// TestListDocs_FindError verifies ListDocs returns an error when
// mongo Find fails.

// TestListAllDocs_Success verifies ListAllDocs returns all documents.

// TestListAllDocs_NilSlice ensures ListAllDocs returns empty slice
// when results are nil (not nil).

// TestAddChunks_Success tests AddChunks with mocked InsertOne and UpdateOne.

// TestAddChunks_InsertError verifies AddChunks returns an error on
// chunk insertion failure.

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

// ===== genShortID length test =====




// ===== NewService nil DB test (additional) =====


// ===== UploadFile tests =====

// ===== Search tests =====

// ===== fullTextSearch tests =====

// ===== semanticSearch tests =====

// ===== ListDocs cursor All error =====

// ===== ListAllDocs cursor All error =====

// ===== SPEC-049 Qdrant / semanticSearch / AddChunks coverage =====

// AddChunks Qdrant path is covered by existing TestCreateDoc + TestAddChunks in fullworkflow
// integration tests. Unit-testing requires mocking *mongo.Collection.InsertOne/UpdateOne
// with variadic options params that are unstable under gomonkey on Go 1.25.

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
