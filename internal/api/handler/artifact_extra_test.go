package handler

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/domain/artifact"
	mockartifact_svc "github.com/luoxiaojun1992/data-agent/internal/service/artifact/mocks"
	mockwm "github.com/luoxiaojun1992/data-agent/internal/logic/workspace/mocks"
)

func newArtifactHandlerForTest(t *testing.T) (*ArtifactHandler, *mockartifact_svc.Service) {
	storage := mockartifact_svc.NewService(t)
	wm := mockwm.NewWorkspaceManager(t)
	return NewArtifactHandler(storage, wm), storage
}

// ── Download 500 branch ──────────────────────────────────────────────

func TestDownload_StorageError(t *testing.T) {
	h, storage := newArtifactHandlerForTest(t)
	storage.On("FindByID", mock.Anything).Return(&artifact.Artifact{ID: "a1", Name: "f.txt", MimeType: "text/plain"}, nil)
	storage.On("Download", mock.Anything).Return(nil, fmt.Errorf("read error"))

	c, w := newGinContext("GET", "/artifacts/a1", "")
	c.Params = gin.Params{{Key: "id", Value: "a1"}}
	h.Download(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── DownloadURL ──────────────────────────────────────────────────────

func TestDownloadURL_Success(t *testing.T) {
	h, storage := newArtifactHandlerForTest(t)
	storage.On("DownloadURL", "a1").Return("/files/artifacts/u/s/f.txt", nil)

	c, w := newGinContext("GET", "/artifacts/a1/url", "")
	c.Params = gin.Params{{Key: "id", Value: "a1"}}
	h.DownloadURL(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "/files/artifacts/u/s/f.txt") {
		t.Errorf("body should contain url: %s", w.Body.String())
	}
}

func TestDownloadURL_NotFound(t *testing.T) {
	h, storage := newArtifactHandlerForTest(t)
	storage.On("DownloadURL", "a1").Return("", fmt.Errorf("not found"))

	c, w := newGinContext("GET", "/artifacts/a1/url", "")
	c.Params = gin.Params{{Key: "id", Value: "a1"}}
	h.DownloadURL(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ── ListUser ─────────────────────────────────────────────────────────

func TestListUser_Success(t *testing.T) {
	h, storage := newArtifactHandlerForTest(t)
	arts := []*artifact.Artifact{{ID: "a1", Name: "f.txt"}}
	storage.On("ListByUser", "user-1", 1, 20).Return(arts, int64(1), nil)

	c, w := newGinContext("GET", "/artifacts?page=1&page_size=20", "")
	c.Set("user_id", "user-1")
	h.ListUser(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "f.txt") {
		t.Errorf("body should contain f.txt: %s", w.Body.String())
	}
}

func TestListUser_StorageError(t *testing.T) {
	h, storage := newArtifactHandlerForTest(t)
	storage.On("ListByUser", "user-1", 1, 20).Return(nil, int64(0), fmt.Errorf("db down"))

	c, w := newGinContext("GET", "/artifacts", "")
	c.Set("user_id", "user-1")
	h.ListUser(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}
