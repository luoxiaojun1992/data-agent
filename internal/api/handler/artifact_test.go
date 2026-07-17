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
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/artifact"
	"github.com/luoxiaojun1992/data-agent/internal/logic/workspace"
	artifact_svc "github.com/luoxiaojun1992/data-agent/internal/service/artifact"
)

func init() { gin.SetMode(gin.TestMode) }

// newMultipartCtx creates a gin context with a multipart file upload request.
func newMultipartCtx(filename, fileContent, sessionID, persistent string) (*gin.Context, *httptest.ResponseRecorder) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", filename)
	io.Copy(part, strings.NewReader(fileContent))
	if sessionID != "" {
		writer.WriteField("session_id", sessionID)
	}
	if persistent != "" {
		writer.WriteField("persistent", persistent)
	}
	writer.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/artifacts", body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	return c, w
}

// ── NewArtifactHandler ──

func TestNewArtifactHandler(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)
	if h == nil {
		t.Fatal("NewArtifactHandler returned nil")
	}
	if h.storage != storage {
		t.Error("storage not set correctly")
	}
	if h.wm != wm {
		t.Error("wm not set correctly")
	}
}

// ── Upload ──

func TestUpload_Success(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	now := time.Now()
	mockArt := &artifact.Artifact{
		ID:        "artifact_1",
		Name:      "test.txt",
		MimeType:  "text/plain",
		SizeBytes: 13,
		UserID:    "user-1",
		SessionID: "sess-1",
		CreatedAt: now,
		UpdatedAt: now,
	}

	patches := gomonkey.ApplyMethodReturn(storage, "Upload", mockArt, nil)
	defer patches.Reset()

	c, w := newMultipartCtx("test.txt", "Hello, World!", "sess-1", "true")
	c.Set("user_id", "user-1")
	h.Upload(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "artifact_1") {
		t.Errorf("body should contain artifact_1: %s", w.Body.String())
	}
}

func TestUpload_NoFile(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	c, w := newGinContext("POST", "/artifacts", "")
	c.Set("user_id", "user-1")
	h.Upload(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestUpload_StorageError(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	patches := gomonkey.ApplyMethodReturn(storage, "Upload", nil, fmt.Errorf("storage full"))
	defer patches.Reset()

	c, w := newMultipartCtx("bigfile.bin", "content", "sess-1", "")
	c.Set("user_id", "user-1")
	h.Upload(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestUpload_WithoutSessionID(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	mockArt := &artifact.Artifact{
		ID:       "artifact_no_session",
		Name:     "test.txt",
		MimeType: "text/plain",
	}

	patches := gomonkey.ApplyMethodReturn(storage, "Upload", mockArt, nil)
	defer patches.Reset()

	// File upload without optional fields
	c, w := newMultipartCtx("test.txt", "data", "", "")
	c.Set("user_id", "user-1")
	h.Upload(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

// ── Download ──

func TestDownload_Success(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	mockArt := &artifact.Artifact{
		ID:       "artifact_1",
		Name:     "report.pdf",
		MimeType: "application/pdf",
	}
	fileData := []byte("PDF content")

	patches := gomonkey.ApplyMethodReturn(storage, "Download", fileData, mockArt, nil)
	defer patches.Reset()

	c, w := newGinContext("GET", "/artifacts/artifact_1", "")
	c.Params = gin.Params{{Key: "id", Value: "artifact_1"}}
	h.Download(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Equal(w.Body.Bytes(), fileData) {
		t.Errorf("body mismatch")
	}
	contentDisposition := w.Header().Get("Content-Disposition")
	if !strings.Contains(contentDisposition, "report.pdf") {
		t.Errorf("Content-Disposition should contain filename: %s", contentDisposition)
	}
}

func TestDownload_NotFound(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	patches := gomonkey.ApplyMethodReturn(storage, "Download", nil, nil, fmt.Errorf("not found"))
	defer patches.Reset()

	c, w := newGinContext("GET", "/artifacts/missing", "")
	c.Params = gin.Params{{Key: "id", Value: "missing"}}
	h.Download(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ── Delete ──

func TestDelete_Success(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	patches := gomonkey.ApplyMethodReturn(storage, "Delete", nil)
	defer patches.Reset()

	c, w := newGinContext("DELETE", "/artifacts/artifact_1", "")
	c.Params = gin.Params{{Key: "id", Value: "artifact_1"}}
	h.Delete(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "deleted") {
		t.Errorf("body should contain deleted: %s", w.Body.String())
	}
}

func TestDelete_StorageError(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	patches := gomonkey.ApplyMethodReturn(storage, "Delete", fmt.Errorf("seaweedfs error"))
	defer patches.Reset()

	c, w := newGinContext("DELETE", "/artifacts/artifact_1", "")
	c.Params = gin.Params{{Key: "id", Value: "artifact_1"}}
	h.Delete(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── ListSession ──

func TestListSession_Success(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	arts := []artifact.Artifact{
		{ID: "a1", Name: "file1.txt"},
		{ID: "a2", Name: "file2.txt"},
	}

	patches := gomonkey.ApplyMethodReturn(storage, "ListBySession", arts, nil)
	defer patches.Reset()

	c, w := newGinContext("GET", "/artifacts?session_id=sess-1", "")
	h.ListSession(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "file1.txt") {
		t.Errorf("body should contain file1.txt: %s", w.Body.String())
	}
}

func TestListSession_MissingSessionID(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	c, w := newGinContext("GET", "/artifacts", "")
	h.ListSession(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListSession_StorageError(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	patches := gomonkey.ApplyMethodReturn(storage, "ListBySession", nil, fmt.Errorf("db error"))
	defer patches.Reset()

	c, w := newGinContext("GET", "/artifacts?session_id=sess-1", "")
	h.ListSession(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestListSession_Empty(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	patches := gomonkey.ApplyMethodReturn(storage, "ListBySession", []artifact.Artifact{}, nil)
	defer patches.Reset()

	c, w := newGinContext("GET", "/artifacts?session_id=sess-empty", "")
	h.ListSession(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ── ListWorkspace ──

func TestListWorkspace_Success(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	files := []string{"report.xlsx", "data.csv"}

	patches := gomonkey.ApplyMethodReturn(wm, "List", files, nil)
	defer patches.Reset()

	c, w := newGinContext("GET", "/workspace/sess-1", "")
	c.Set("user_id", "user-1")
	c.Params = gin.Params{{Key: "session_id", Value: "sess-1"}}
	h.ListWorkspace(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "report.xlsx") {
		t.Errorf("body should contain report.xlsx: %s", w.Body.String())
	}
}

func TestListWorkspace_Error(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	patches := gomonkey.ApplyMethodReturn(wm, "List", nil, fmt.Errorf("workspace error"))
	defer patches.Reset()

	c, w := newGinContext("GET", "/workspace/sess-1", "")
	c.Set("user_id", "user-1")
	c.Params = gin.Params{{Key: "session_id", Value: "sess-1"}}
	h.ListWorkspace(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestListWorkspace_Empty(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	patches := gomonkey.ApplyMethodReturn(wm, "List", []string{}, nil)
	defer patches.Reset()

	c, w := newGinContext("GET", "/workspace/sess-1", "")
	c.Set("user_id", "user-1")
	c.Params = gin.Params{{Key: "session_id", Value: "sess-1"}}
	h.ListWorkspace(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ── ReadWorkspaceFile ──

func TestReadWorkspaceFile_Success(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	data := []byte("workspace file content")

	patches := gomonkey.ApplyMethodReturn(wm, "ReadFile", data, nil)
	defer patches.Reset()

	c, w := newGinContext("GET", "/workspace/sess-1/read/file.txt", "")
	c.Set("user_id", "user-1")
	c.Params = gin.Params{
		{Key: "session_id", Value: "sess-1"},
		{Key: "filename", Value: "file.txt"},
	}
	h.ReadWorkspaceFile(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Equal(w.Body.Bytes(), data) {
		t.Errorf("body mismatch")
	}
}

func TestReadWorkspaceFile_NotFound(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	patches := gomonkey.ApplyMethodReturn(wm, "ReadFile", nil, fmt.Errorf("not found"))
	defer patches.Reset()

	c, w := newGinContext("GET", "/workspace/sess-1/read/missing.txt", "")
	c.Set("user_id", "user-1")
	c.Params = gin.Params{
		{Key: "session_id", Value: "sess-1"},
		{Key: "filename", Value: "missing.txt"},
	}
	h.ReadWorkspaceFile(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ── WriteWorkspaceFile ──

func TestWriteWorkspaceFile_Success(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	patches := gomonkey.ApplyMethodReturn(wm, "WriteFile", nil)
	defer patches.Reset()

	body := "new file data"
	c, w := newGinContext("POST", "/workspace/sess-1/write/newfile.txt", body)
	c.Set("user_id", "user-1")
	c.Params = gin.Params{
		{Key: "session_id", Value: "sess-1"},
		{Key: "filename", Value: "newfile.txt"},
	}
	h.WriteWorkspaceFile(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "written") {
		t.Errorf("body should contain written: %s", w.Body.String())
	}
}

func TestWriteWorkspaceFile_Error(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	patches := gomonkey.ApplyMethodReturn(wm, "WriteFile", fmt.Errorf("disk full"))
	defer patches.Reset()

	c, w := newGinContext("POST", "/workspace/sess-1/write/file.txt", "data")
	c.Set("user_id", "user-1")
	c.Params = gin.Params{
		{Key: "session_id", Value: "sess-1"},
		{Key: "filename", Value: "file.txt"},
	}
	h.WriteWorkspaceFile(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestWriteWorkspaceFile_EmptyBody(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	patches := gomonkey.ApplyMethodReturn(wm, "WriteFile", nil)
	defer patches.Reset()

	c, w := newGinContext("POST", "/workspace/sess-1/write/empty.txt", "")
	c.Set("user_id", "user-1")
	c.Params = gin.Params{
		{Key: "session_id", Value: "sess-1"},
		{Key: "filename", Value: "empty.txt"},
	}
	h.WriteWorkspaceFile(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ── Upload Missing File Field ──

func TestUpload_EmptyContentType(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	mockArt := &artifact.Artifact{
		ID:        "artifact_empty_ct",
		Name:      "test.dat",
		MimeType:  "application/octet-stream",
		SizeBytes: 10,
		UserID:    "user-1",
		SessionID: "sess-1",
	}

	patches := gomonkey.ApplyMethodReturn(storage, "Upload", mockArt, nil)
	defer patches.Reset()

	// Build multipart form where file part has filename but NO Content-Type
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	hdr := make(map[string][]string)
	hdr["Content-Disposition"] = []string{`form-data; name="file"; filename="test.dat"`}
	part, _ := writer.CreatePart(hdr)
	io.Copy(part, strings.NewReader("hello data"))
	writer.Close()

	c, w := newGinContext("POST", "/artifacts", "")
	c.Request = httptest.NewRequest("POST", "/artifacts", body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	c.Set("user_id", "user-1")

	h.Upload(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpload_MissingFileField(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("session_id", "sess-1")
	writer.WriteField("persistent", "true")
	writer.Close()

	c, w := newGinContext("POST", "/artifacts", "")
	// Override with multipart request
	c.Request = httptest.NewRequest("POST", "/artifacts", body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	c.Set("user_id", "user-1")

	h.Upload(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when file field missing, got %d: %s", w.Code, w.Body.String())
	}
}

// ── WriteWorkspaceFile ReadError ──

func TestWriteWorkspaceFile_ReadError(t *testing.T) {
	storage := &artifact_svc.Storage{}
	wm := &workspace.Manager{}
	h := NewArtifactHandler(storage, wm)

	patches := gomonkey.ApplyFuncReturn(io.ReadAll, []byte(nil), fmt.Errorf("read error"))
	defer patches.Reset()

	c, w := newGinContext("POST", "/workspace/sess-1/write/file.txt", "data")
	c.Set("user_id", "user-1")
	c.Params = gin.Params{
		{Key: "session_id", Value: "sess-1"},
		{Key: "filename", Value: "file.txt"},
	}
	h.WriteWorkspaceFile(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when body read fails, got %d: %s", w.Code, w.Body.String())
	}
}
