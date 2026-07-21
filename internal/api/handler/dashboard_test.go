package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	chatmocks "github.com/luoxiaojun1992/data-agent/internal/domain/chat/mocks"
	domainknowledge "github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	taskmocks "github.com/luoxiaojun1992/data-agent/internal/domain/task/mocks"
	kbmocks "github.com/luoxiaojun1992/data-agent/internal/service/knowledge/mocks"
)

func TestDashboardHandler_Get(t *testing.T) {
	tasks := taskmocks.NewTaskService(t)
	sessions := chatmocks.NewSessionService(t)
	kb := kbmocks.NewKnowledgeService(t)

	tasks.On("ListAllTasks", "u1").Return([]*domaintask.Task{{ID: "t1"}}, nil)
	sessions.On("ListByUser", "u1").Return([]*domainchat.Session{{ID: "s1"}}, nil)
	kb.On("ListAllDocs").Return([]*domainknowledge.KnowledgeDoc{{ID: "d1"}}, nil)

	h := NewDashboardHandler(tasks, sessions, kb)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", "u1")
	c.Request = httptest.NewRequest("GET", "/dashboard", nil)
	h.Get(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["tasks"] == nil || resp["sessions"] == nil || resp["docs"] == nil {
		t.Errorf("missing fields: %+v", resp)
	}
}

func TestNewDashboardHandler(t *testing.T) {
	h := NewDashboardHandler(nil, nil, nil)
	if h == nil {
		t.Error("handler should not be nil")
	}
}

var _ = mock.Anything
