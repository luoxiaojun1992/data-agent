package handler

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	knowledgesvc "github.com/luoxiaojun1992/data-agent/internal/service/knowledge"
	mocksvc "github.com/luoxiaojun1992/data-agent/internal/service/knowledge/mocks"
)

func TestSetPublicFlag_Success(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)
	svc.On("SetPublicFlag", mock.Anything, "doc_1", true, "u1", false).Return(nil)

	c, w := newGinContext("PATCH", "/knowledge/docs/doc_1/public", `{"is_public":true}`)
	c.Params = gin.Params{{Key: "id", Value: "doc_1"}}
	c.Set("user_id", "u1")
	c.Set("role", "user")
	h.SetPublicFlag(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetPublicFlag_NotFound(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)
	svc.On("SetPublicFlag", mock.Anything, "doc_missing", true, "u1", false).Return(knowledgesvc.ErrNotFound)

	c, w := newGinContext("PATCH", "/knowledge/docs/doc_missing/public", `{"is_public":true}`)
	c.Params = gin.Params{{Key: "id", Value: "doc_missing"}}
	c.Set("user_id", "u1")
	c.Set("role", "user")
	h.SetPublicFlag(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSetPublicFlag_InternalError(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)
	svc.On("SetPublicFlag", mock.Anything, "doc_1", true, "u1", false).Return(fmt.Errorf("db down"))

	c, w := newGinContext("PATCH", "/knowledge/docs/doc_1/public", `{"is_public":true}`)
	c.Params = gin.Params{{Key: "id", Value: "doc_1"}}
	c.Set("user_id", "u1")
	c.Set("role", "user")
	h.SetPublicFlag(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestSetPublicFlag_BadJSON(t *testing.T) {
	svc := mocksvc.NewKnowledgeService(t)
	h := NewKnowledgeHandler(svc)

	c, w := newGinContext("PATCH", "/knowledge/docs/doc_1/public", "not-json")
	c.Params = gin.Params{{Key: "id", Value: "doc_1"}}
	c.Set("user_id", "u1")
	h.SetPublicFlag(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	svc.AssertNotCalled(t, "SetPublicFlag", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
