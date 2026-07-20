package handler

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	mocksvc "github.com/luoxiaojun1992/data-agent/internal/service/knowledge/mocks"
)

func init() { gin.SetMode(gin.TestMode) }

// newKnowledgeMultipartCtx creates a context with a multipart file upload for knowledge.
func newKnowledgeMultipartCtx(filename, content string, fields map[string]string) (*gin.Context, *httptest.ResponseRecorder) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if filename != "" {
		part, _ := writer.CreateFormFile("file", filename)
		_, _ = io.Copy(part, strings.NewReader(content))
	}
	for k, v := range fields {
		_ = writer.WriteField(k, v)
	}
	writer.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/knowledge/docs", body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	return c, w
}

// ── NewKnowledgeHandler ──

func TestNewKnowledgeHandler(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)
	if h == nil {
		t.Fatal("NewKnowledgeHandler returned nil")
	}
	if h.svc != svc {
		t.Error("svc not set correctly")
	}
}

// ── UploadDoc ──

func TestUploadDoc_Success(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	mockDoc := &knowledge.KnowledgeDoc{
		ID:       "kbdoc_1",
		UserID:   "user-1",
		Title:    "Test Doc",
		FileName: "test.pdf",
		FileType: "pdf",
		Status:   knowledge.StatusUploaded,
	}

	svc.On("UploadFile", mock.Anything, mock.Anything, mock.Anything).Return("gridfs_1", nil)
	svc.On("CreateDoc", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mockDoc, nil)

	c, w := newKnowledgeMultipartCtx("test.pdf", "PDF content", map[string]string{
		"title":     "Test Doc",
		"file_name": "test.pdf",
		"file_type": "pdf",
	})
	c.Set("user_id", "user-1")
	h.UploadDoc(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUploadDoc_NoFile_Success(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	mockDoc := &knowledge.KnowledgeDoc{
		ID:     "kbdoc_2",
		UserID: "user-1",
		Title:  "Metadata Only",
		Status: knowledge.StatusUploaded,
	}

	svc.On("CreateDoc", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mockDoc, nil)

	// No file, just metadata
	c, w := newKnowledgeMultipartCtx("", "", map[string]string{
		"title":     "Metadata Only",
		"file_name": "notes.txt",
	})
	c.Set("user_id", "user-1")
	h.UploadDoc(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUploadDoc_GridFSUploadError(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	svc.On("UploadFile", mock.Anything, mock.Anything, mock.Anything).Return("", fmt.Errorf("gridfs full"))

	c, w := newKnowledgeMultipartCtx("large.pdf", "content", map[string]string{
		"title":     "Large Doc",
		"file_name": "large.pdf",
		"file_type": "pdf",
	})
	c.Set("user_id", "user-1")
	h.UploadDoc(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestUploadDoc_CreateDocError(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	svc.On("UploadFile", mock.Anything, mock.Anything, mock.Anything).Return("gridfs_1", nil)
	svc.On("CreateDoc", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("insert failed"))

	c, w := newKnowledgeMultipartCtx("test.pdf", "content", map[string]string{
		"title":     "Test Doc",
		"file_name": "test.pdf",
	})
	c.Set("user_id", "user-1")
	h.UploadDoc(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestUploadDoc_WithSizeBytes(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	mockDoc := &knowledge.KnowledgeDoc{ID: "kbdoc_3", UserID: "user-1", Title: "Sized Doc"}

	svc.On("UploadFile", mock.Anything, mock.Anything, mock.Anything).Return("gridfs_2", nil)
	svc.On("CreateDoc", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mockDoc, nil)

	c, w := newKnowledgeMultipartCtx("doc.txt", "hello", map[string]string{
		"title":      "Sized Doc",
		"file_name":  "doc.txt",
		"file_type":  "txt",
		"size_bytes": "1024",
	})
	c.Set("user_id", "user-1")
	h.UploadDoc(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

// ── GetDoc ──

func TestGetDoc_Success(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	mockDoc := &knowledge.KnowledgeDoc{
		ID:       "kbdoc_1",
		Title:    "My Document",
		UserID:   "user-1",
		FileName: "doc.pdf",
		Status:   knowledge.StatusReady,
	}

	svc.On("GetDoc", mock.Anything).Return(mockDoc, nil)

	c, w := newGinContext("GET", "/knowledge/docs/kbdoc_1", "")
	c.Params = gin.Params{{Key: "id", Value: "kbdoc_1"}}
	h.GetDoc(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetDoc_NotFound(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	svc.On("GetDoc", mock.Anything).Return(nil, fmt.Errorf("not found"))

	c, w := newGinContext("GET", "/knowledge/docs/missing", "")
	c.Params = gin.Params{{Key: "id", Value: "missing"}}
	h.GetDoc(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ── DeleteDoc ──

func TestDeleteDoc_Success(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	svc.On("DeleteDoc", mock.Anything).Return(nil)

	c, w := newGinContext("DELETE", "/knowledge/docs/kbdoc_1", "")
	c.Params = gin.Params{{Key: "id", Value: "kbdoc_1"}}
	h.DeleteDoc(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "deleted") {
		t.Errorf("body should contain deleted: %s", w.Body.String())
	}
}

func TestDeleteDoc_Error(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	svc.On("DeleteDoc", mock.Anything).Return(fmt.Errorf("db error"))

	c, w := newGinContext("DELETE", "/knowledge/docs/kbdoc_1", "")
	c.Params = gin.Params{{Key: "id", Value: "kbdoc_1"}}
	h.DeleteDoc(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── ListDocs ──

func TestListDocs_Success(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	docs := []*knowledge.KnowledgeDoc{
		{ID: "kbdoc_1", Title: "Doc 1", UserID: "user-1"},
		{ID: "kbdoc_2", Title: "Doc 2", UserID: "user-1"},
	}

	svc.On("ListDocs", mock.Anything).Return(docs, nil)

	c, w := newGinContext("GET", "/knowledge/docs", "")
	c.Set("user_id", "user-1")
	h.ListDocs(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListDocs_Error(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	svc.On("ListDocs", mock.Anything).Return(nil, fmt.Errorf("db error"))

	c, w := newGinContext("GET", "/knowledge/docs", "")
	c.Set("user_id", "user-1")
	h.ListDocs(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestListDocs_Empty(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	svc.On("ListDocs", mock.Anything).Return([]*knowledge.KnowledgeDoc{}, nil)

	c, w := newGinContext("GET", "/knowledge/docs", "")
	c.Set("user_id", "user-1")
	h.ListDocs(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ── Search ──

func TestSearch_Success(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	results := []knowledge.SearchResult{
		{ChunkID: "chunk_1", DocID: "kbdoc_1", DocTitle: "Doc 1", Content: "relevant content", Score: 0.9, Source: "fulltext"},
	}

	svc.On("Search", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(results, nil)

	c, w := newGinContext("GET", "/knowledge/search?q=relevant", "")
	c.Set("user_id", "user-1")
	c.Set("role", "user")
	h.Search(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "relevant") {
		t.Errorf("body should contain relevant: %s", w.Body.String())
	}
}

func TestSearch_MissingQuery(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	c, w := newGinContext("GET", "/knowledge/search", "")
	c.Set("user_id", "user-1")
	c.Set("role", "user")
	h.Search(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	c, w := newGinContext("GET", "/knowledge/search?q=", "")
	c.Set("user_id", "user-1")
	h.Search(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSearch_ServiceError(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	svc.On("Search", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("search failed"))

	c, w := newGinContext("GET", "/knowledge/search?q=test", "")
	c.Set("user_id", "user-1")
	c.Set("role", "user")
	h.Search(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestSearch_NoResults(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	svc.On("Search", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]knowledge.SearchResult{}, nil)

	c, w := newGinContext("GET", "/knowledge/search?q=nothing", "")
	c.Set("user_id", "user-1")
	c.Set("role", "user")
	h.Search(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ── AddChunks ──

func TestAddChunks_Success(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	svc.On("AddChunks", mock.Anything, mock.Anything).Return(nil)

	body := `{"chunks":["chunk 1 content","chunk 2 content","chunk 3 content"]}`
	c, w := newGinContext("POST", "/knowledge/docs/kbdoc_1/chunks", body)
	c.Params = gin.Params{{Key: "id", Value: "kbdoc_1"}}
	h.AddChunks(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "indexed") {
		t.Errorf("body should contain indexed: %s", w.Body.String())
	}
}

func TestAddChunks_InvalidJSON(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	c, w := newGinContext("POST", "/knowledge/docs/kbdoc_1/chunks", "bad")
	c.Params = gin.Params{{Key: "id", Value: "kbdoc_1"}}
	h.AddChunks(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAddChunks_ServiceError(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	svc.On("AddChunks", mock.Anything, mock.Anything).Return(fmt.Errorf("chunk insert failed"))

	body := `{"chunks":["chunk 1"]}`
	c, w := newGinContext("POST", "/knowledge/docs/kbdoc_1/chunks", body)
	c.Params = gin.Params{{Key: "id", Value: "kbdoc_1"}}
	h.AddChunks(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestAddChunks_EmptyChunks(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	svc.On("AddChunks", mock.Anything, mock.Anything).Return(nil)

	body := `{"chunks":[]}`
	c, w := newGinContext("POST", "/knowledge/docs/kbdoc_1/chunks", body)
	c.Params = gin.Params{{Key: "id", Value: "kbdoc_1"}}
	h.AddChunks(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ── ListAllDocs ──

func TestListAllDocs_Success(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	docs := []*knowledge.KnowledgeDoc{
		{ID: "kbdoc_1", Title: "Doc 1", UserID: "user-1"},
		{ID: "kbdoc_2", Title: "Doc 2", UserID: "user-2"},
	}

	svc.On("ListAllDocs").Return(docs, nil)

	c, w := newGinContext("GET", "/knowledge/admin/docs", "")
	h.ListAllDocs(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListAllDocs_Error(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	svc.On("ListAllDocs").Return(nil, fmt.Errorf("db error"))

	c, w := newGinContext("GET", "/knowledge/admin/docs", "")
	h.ListAllDocs(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestListAllDocs_Empty(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	svc.On("ListAllDocs").Return([]*knowledge.KnowledgeDoc{}, nil)

	c, w := newGinContext("GET", "/knowledge/admin/docs", "")
	h.ListAllDocs(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
