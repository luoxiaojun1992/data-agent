package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// fakeMemoryService implements memory.Service for handler testing.
type fakeMemoryService struct {
	entries []memory.Entry
	err     error
}

func (f *fakeMemoryService) AddSessionToMemory(ctx context.Context, s session.Session) error {
	return nil
}

func (f *fakeMemoryService) SearchMemory(ctx context.Context, req *memory.SearchRequest) (*memory.SearchResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &memory.SearchResponse{Memories: f.entries}, nil
}

func TestMemoryHandler_Search_Success(t *testing.T) {
	svc := &fakeMemoryService{
		entries: []memory.Entry{
			{ID: "m1", Content: genai.NewContentFromText("记忆内容1", "model")},
			{ID: "m2", Content: genai.NewContentFromText("记忆内容2", "model")},
		},
	}
	h := NewMemoryHandler(svc, "data-agent")
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/memory/search?q=营收&user_id=u1", nil)
	h.Search(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !contains(body, "记忆内容1") || !contains(body, "记忆内容2") {
		t.Errorf("missing memory text in response: %s", body)
	}
	if !contains(body, `"count":2`) {
		t.Errorf("missing or wrong count: %s", body)
	}
}

func TestMemoryHandler_Search_Error(t *testing.T) {
	svc := &fakeMemoryService{err: errStr("memory backend down")}
	h := NewMemoryHandler(svc, "data-agent")
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/memory/search?q=test&user_id=u1", nil)
	h.Search(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestMemoryHandler_Search_UsesContextUserID(t *testing.T) {
	svc := &fakeMemoryService{
		entries: []memory.Entry{
			{Content: genai.NewContentFromText("ctx user memory", "model")},
		},
	}
	h := NewMemoryHandler(svc, "data-agent")
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", "ctx-user")
	c.Request = httptest.NewRequest("GET", "/memory/search?q=test", nil)
	h.Search(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !contains(w.Body.String(), "ctx user memory") {
		t.Errorf("should use context user_id: %s", w.Body.String())
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && indexOf(s, substr) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
