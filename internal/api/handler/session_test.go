package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	chatmocks "github.com/luoxiaojun1992/data-agent/internal/domain/chat/mocks"
	"google.golang.org/adk/model"
	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"
)

func newSessionGin(method, path string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(""))
	return c, w
}

func TestSessionHandler_List(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("ListByUserPaged", "u1", "", 1, 15).Return([]*domainchat.Session{{ID: "s1"}}, int64(1), nil)
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("GET", "/sessions")
	c.Set("user_id", "u1")
	h.List(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	sessions, _ := resp["sessions"].([]any)
	if len(sessions) != 1 {
		t.Errorf("sessions = %v", sessions)
	}
}

func TestSessionHandler_List_WithQ(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	// q 应原样透传到 service 层（SPEC-075：DB 层过滤）
	mgr.On("ListByUserPaged", "u1", "销售", 1, 15).Return([]*domainchat.Session{{ID: "s1"}}, int64(1), nil)
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("GET", "/sessions?q=%E9%94%80%E5%94%AE")
	c.Set("user_id", "u1")
	h.List(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSessionHandler_Create(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Create", "u1", "chat", "").Return(&domainchat.Session{ID: "s2", ExpiresAt: time.Now().Add(time.Hour)}, nil)
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("POST", "/sessions?type=chat")
	c.Set("user_id", "u1")
	h.Create(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["session_id"] != "s2" {
		t.Errorf("session_id = %v", resp["session_id"])
	}
}

func TestSessionHandler_Create_Error(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Create", "u1", "chat", "").Return((*domainchat.Session)(nil), errStr("db"))
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("POST", "/sessions")
	c.Set("user_id", "u1")
	h.Create(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestSessionHandler_Get(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Get", "s1").Return(&domainchat.Session{ID: "s1"}, nil)
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("GET", "/sessions/s1")
	c.Params = gin.Params{{Key: "id", Value: "s1"}}
	h.Get(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSessionHandler_Get_NotFound(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Get", "missing").Return((*domainchat.Session)(nil), errStr("not found"))
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("GET", "/sessions/missing")
	c.Params = gin.Params{{Key: "id", Value: "missing"}}
	h.Get(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSessionHandler_Renew(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Renew", "s1").Return(nil)
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("PUT", "/sessions/s1")
	c.Params = gin.Params{{Key: "id", Value: "s1"}}
	h.Renew(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSessionHandler_Delete(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Delete", "s1").Return(nil)
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("DELETE", "/sessions/s1")
	c.Params = gin.Params{{Key: "id", Value: "s1"}}
	h.Delete(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSessionHandler_Restore(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Restore", "s1").Return(nil)
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("POST", "/sessions/s1/restore")
	c.Params = gin.Params{{Key: "id", Value: "s1"}}
	h.Restore(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSessionHandler_ListDeleted(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("ListDeleted", mock.Anything, mock.Anything).Return([]*domainchat.Session{
		{ID: "d1", UserID: "u1"},
		{ID: "d2", UserID: "other"},
	}, nil)
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("GET", "/sessions/deleted")
	c.Set("user_id", "u1")
	h.ListDeleted(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	sessions, _ := resp["sessions"].([]any)
	if len(sessions) != 1 {
		t.Errorf("expected 1 user session, got %d", len(sessions))
	}
}

func TestSessionHandler_ListDeleted_Error(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("ListDeleted", mock.Anything, mock.Anything).Return(([]*domainchat.Session)(nil), errStr("db"))
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("GET", "/sessions/deleted")
	c.Set("user_id", "u1")
	h.ListDeleted(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestSessionHandler_MessagesCanonicalTranscript(t *testing.T) {
	ctx := context.Background()
	adk := adksession.InMemoryService()
	created, err := adk.Create(ctx, &adksession.CreateRequest{
		AppName: "data-agent", UserID: "u1", SessionID: "s1",
	})
	if err != nil {
		t.Fatalf("create ADK session: %v", err)
	}
	base := time.Date(2026, 7, 26, 22, 0, 0, 0, time.UTC)
	appendEvent := func(id, author string, content *genai.Content) {
		t.Helper()
		if err := adk.AppendEvent(ctx, created.Session, &adksession.Event{
			ID: id, Author: author, Timestamp: base,
			LLMResponse: model.LLMResponse{Content: content},
		}); err != nil {
			t.Fatalf("append event %s: %v", id, err)
		}
	}
	appendEvent("user-1", "user", genai.NewContentFromText("查询销售", "user"))
	appendEvent("assistant-1", "data_agent", genai.NewContentFromText("我来", "model"))
	appendEvent("assistant-2", "data_agent", genai.NewContentFromText("查询", "model"))
	appendEvent("tool-call-1", "data_agent", &genai.Content{Role: "model", Parts: []*genai.Part{{
		FunctionCall: &genai.FunctionCall{Name: "sql_query", Args: map[string]any{"sql": "SELECT 1"}},
	}}})
	appendEvent("tool-result-1", "data_agent", &genai.Content{Role: "user", Parts: []*genai.Part{{
		FunctionResponse: &genai.FunctionResponse{Name: "sql_query", Response: map[string]any{"rows": []any{"row-1"}}},
	}}})
	appendEvent("assistant-3", "data_agent", genai.NewContentFromText("完成", "model"))
	appendEvent("compaction-1", "compaction", genai.NewContentFromText("should not be shown", "model"))

	h := NewSessionHandler(nil, adk)
	c, w := newSessionGin("GET", "/sessions/s1/messages")
	c.Set("user_id", "u1")
	c.Params = gin.Params{{Key: "id", Value: "s1"}}
	h.Messages(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Messages []domainchat.ChatEvent `json:"messages"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Messages) != 7 {
		t.Fatalf("messages = %d, want 7: %+v", len(body.Messages), body.Messages)
	}
	if body.Messages[0].Role != "user" || body.Messages[0].Content != "查询销售" {
		t.Errorf("user message = %+v", body.Messages[0])
	}
	if body.Messages[1].Type != "text" || body.Messages[1].Content != "我来" {
		t.Errorf("assistant text #1 = %+v", body.Messages[1])
	}
	if body.Messages[2].Type != "text" || body.Messages[2].Content != "查询" {
		t.Errorf("assistant text #2 = %+v", body.Messages[2])
	}
	if body.Messages[3].Type != "tool_call" || body.Messages[3].Name != "sql_query" {
		t.Errorf("tool call = %+v", body.Messages[3])
	}
	if body.Messages[4].Type != "tool_result" || body.Messages[4].Result == nil {
		t.Errorf("tool result = %+v", body.Messages[4])
	}
	if body.Messages[5].Content != "完成" {
		t.Errorf("final text = %+v", body.Messages[5])
	}
	if body.Messages[6].Role != "system" || body.Messages[6].Content != "[compaction] 上下文已自动压缩" {
		t.Errorf("compaction notice = %+v", body.Messages[6])
	}
	if strings.Contains(w.Body.String(), "should not be shown") {
		t.Error("compaction summary leaked into transcript")
	}
}
