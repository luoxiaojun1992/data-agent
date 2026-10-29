package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
	"github.com/luoxiaojun1992/data-agent/internal/service/apicollection"
)

func newAPICollectionHandler() (*APICollectionHandler, *apicollection.Service) {
	svc := &apicollection.Service{}
	return NewAPICollectionHandler(svc), svc
}

func TestAPICollectionList_NonAdminFiltersByOwner(t *testing.T) {
	h, svc := newAPICollectionHandler()
	var gotOwnerID string
	patches := gomonkey.ApplyMethodFunc(svc, "List",
		func(_ context.Context, ownerID string, page, pageSize int) (*mongo.ListResult, error) {
			gotOwnerID = ownerID
			return &mongo.ListResult{Total: 1}, nil
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("GET", "/api-collections?page=1&page_size=20", "")
	c.Set("user_id", "u1")
	c.Set("role", "user")
	h.List(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotOwnerID != "u1" {
		t.Errorf("ownerID = %q, want u1 (non-admin must be scoped to self)", gotOwnerID)
	}
}

func TestAPICollectionList_SystemAdminSeesAll(t *testing.T) {
	h, svc := newAPICollectionHandler()
	var gotOwnerID string
	patches := gomonkey.ApplyMethodFunc(svc, "List",
		func(_ context.Context, ownerID string, page, pageSize int) (*mongo.ListResult, error) {
			gotOwnerID = ownerID
			return &mongo.ListResult{Total: 0}, nil
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("GET", "/api-collections", "")
	c.Set("user_id", "u1")
	c.Set("role", "system_admin")
	h.List(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotOwnerID != "" {
		t.Errorf("ownerID = %q, want empty (system_admin sees all)", gotOwnerID)
	}
}

func TestAPICollectionList_Error(t *testing.T) {
	h, svc := newAPICollectionHandler()
	patches := gomonkey.ApplyMethodFunc(svc, "List",
		func(_ context.Context, ownerID string, page, pageSize int) (*mongo.ListResult, error) {
			return nil, errors.New("db down")
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("GET", "/api-collections", "")
	c.Set("user_id", "u1")
	c.Set("role", "user")
	h.List(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestAPICollectionGet_NotFound(t *testing.T) {
	h, svc := newAPICollectionHandler()
	patches := gomonkey.ApplyMethodFunc(svc, "Get",
		func(_ context.Context, id, ownerID string) (*model.APICollection, error) {
			return nil, errors.New("not found")
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("GET", "/api-collections/c1", "")
	c.Set("user_id", "u1")
	c.Set("role", "user")
	c.Params = gin.Params{gin.Param{Key: "id", Value: "c1"}}
	h.Get(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAPICollectionApprove_InvalidStatus(t *testing.T) {
	h, _ := newAPICollectionHandler()
	c, w := newGinContext("POST", "/api-collections/c1/approve", `{"status":"bogus"}`)
	c.Params = gin.Params{gin.Param{Key: "id", Value: "c1"}}
	h.Approve(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAPICollectionApprove_ValidStatus(t *testing.T) {
	h, svc := newAPICollectionHandler()
	var gotID string
	var gotStatus model.APICollectionStatus
	patches := gomonkey.ApplyMethodFunc(svc, "Approve",
		func(_ context.Context, id string, status model.APICollectionStatus) error {
			gotID, gotStatus = id, status
			return nil
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("POST", "/api-collections/c1/approve", `{"status":"approved"}`)
	c.Params = gin.Params{gin.Param{Key: "id", Value: "c1"}}
	h.Approve(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotID != "c1" || gotStatus != model.APICollectionApproved {
		t.Errorf("Approve(%q, %q)", gotID, gotStatus)
	}
}

func TestAPICollectionCreate_NoFile(t *testing.T) {
	h, _ := newAPICollectionHandler()
	c, w := newGinContext("POST", "/api-collections", "")
	c.Set("user_id", "u1")
	h.Create(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 'file required', got %d: %s", w.Code, w.Body.String())
	}
}
