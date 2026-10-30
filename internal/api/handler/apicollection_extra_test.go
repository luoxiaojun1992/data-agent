package handler

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
)

func newAPICollUploadCtx(method, path, fieldName, fileName, content string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	part, _ := w.CreateFormFile(fieldName, fileName)
	_, _ = part.Write([]byte(content))
	_ = w.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, path, body)
	c.Request.Header.Set("Content-Type", w.FormDataContentType())
	return c, rec
}

// ── Get ──────────────────────────────────────────────────────────────

func TestAPICollectionGet_OK(t *testing.T) {
	h, svc := newAPICollectionHandler()
	var gotID, gotOwner string
	patches := gomonkey.ApplyMethodFunc(svc, "Get",
		func(_ context.Context, id, ownerID string) (*model.APICollection, error) {
			gotID, gotOwner = id, ownerID
			return &model.APICollection{ID: id}, nil
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("GET", "/api-collections/c1", "")
	c.Set("user_id", "u1")
	c.Set("role", "user")
	c.Params = gin.Params{gin.Param{Key: "id", Value: "c1"}}
	h.Get(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotID != "c1" || gotOwner != "u1" {
		t.Errorf("Get(%q,%q), want (c1,u1)", gotID, gotOwner)
	}
}

// ── Create (multipart upload) ────────────────────────────────────────

func TestAPICollectionCreate_Success(t *testing.T) {
	h, svc := newAPICollectionHandler()
	var gotUserID string
	var gotSpecLen int
	patches := gomonkey.ApplyMethodFunc(svc, "CreateUpload",
		func(_ context.Context, userID, name, description string, rawSpec []byte, fileID string) (*model.APICollection, error) {
			gotUserID, gotSpecLen = userID, len(rawSpec)
			return &model.APICollection{ID: "c1"}, nil
		})
	t.Cleanup(patches.Reset)

	c, w := newAPICollUploadCtx("POST", "/api-collections", "file", "openapi.json", `{"openapi":"3.0"}`)
	c.Set("user_id", "u1")
	h.Create(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if gotUserID != "u1" || gotSpecLen == 0 {
		t.Errorf("CreateUpload userID=%q specLen=%d, want u1 / non-zero", gotUserID, gotSpecLen)
	}
}

func TestAPICollectionCreate_ServiceError(t *testing.T) {
	h, svc := newAPICollectionHandler()
	patches := gomonkey.ApplyMethodFunc(svc, "CreateUpload",
		func(_ context.Context, userID, name, description string, rawSpec []byte, fileID string) (*model.APICollection, error) {
			return nil, errors.New("invalid spec")
		})
	t.Cleanup(patches.Reset)

	c, w := newAPICollUploadCtx("POST", "/api-collections", "file", "openapi.json", `{}`)
	c.Set("user_id", "u1")
	h.Create(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── Update ───────────────────────────────────────────────────────────

func TestAPICollectionUpdate_OK(t *testing.T) {
	h, svc := newAPICollectionHandler()
	var gotID, gotUser, gotName string
	patches := gomonkey.ApplyMethodFunc(svc, "Update",
		func(_ context.Context, id, userID, name, desc string) error {
			gotID, gotUser, gotName = id, userID, name
			return nil
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("PUT", "/api-collections/c1", `{"name":"new","description":"d"}`)
	c.Set("user_id", "u1")
	c.Params = gin.Params{gin.Param{Key: "id", Value: "c1"}}
	h.Update(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotID != "c1" || gotUser != "u1" || gotName != "new" {
		t.Errorf("Update(%q,%q,%q)", gotID, gotUser, gotName)
	}
}

func TestAPICollectionUpdate_BadJSON(t *testing.T) {
	h, _ := newAPICollectionHandler()
	c, w := newGinContext("PUT", "/api-collections/c1", `{bad`)
	c.Params = gin.Params{gin.Param{Key: "id", Value: "c1"}}
	h.Update(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAPICollectionUpdate_ServiceError(t *testing.T) {
	h, svc := newAPICollectionHandler()
	patches := gomonkey.ApplyMethodFunc(svc, "Update",
		func(_ context.Context, id, userID, name, desc string) error { return errors.New("nf") })
	t.Cleanup(patches.Reset)

	c, w := newGinContext("PUT", "/api-collections/c1", `{}`)
	c.Set("user_id", "u1")
	c.Params = gin.Params{gin.Param{Key: "id", Value: "c1"}}
	h.Update(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── Delete ───────────────────────────────────────────────────────────

func TestAPICollectionDelete_OK(t *testing.T) {
	h, svc := newAPICollectionHandler()
	var gotID, gotUser string
	patches := gomonkey.ApplyMethodFunc(svc, "Delete",
		func(_ context.Context, id, userID string) error {
			gotID, gotUser = id, userID
			return nil
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("DELETE", "/api-collections/c1", "")
	c.Set("user_id", "u1")
	c.Params = gin.Params{gin.Param{Key: "id", Value: "c1"}}
	h.Delete(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotID != "c1" || gotUser != "u1" {
		t.Errorf("Delete(%q,%q)", gotID, gotUser)
	}
}

func TestAPICollectionDelete_Error(t *testing.T) {
	h, svc := newAPICollectionHandler()
	patches := gomonkey.ApplyMethodFunc(svc, "Delete",
		func(_ context.Context, id, userID string) error { return errors.New("nf") })
	t.Cleanup(patches.Reset)

	c, w := newGinContext("DELETE", "/api-collections/c1", "")
	c.Set("user_id", "u1")
	c.Params = gin.Params{gin.Param{Key: "id", Value: "c1"}}
	h.Delete(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── Approve service error ────────────────────────────────────────────

func TestAPICollectionApprove_ServiceError(t *testing.T) {
	h, svc := newAPICollectionHandler()
	patches := gomonkey.ApplyMethodFunc(svc, "Approve",
		func(_ context.Context, id string, status model.APICollectionStatus) error {
			return errors.New("db down")
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("POST", "/api-collections/c1/approve", `{"status":"rejected"}`)
	c.Params = gin.Params{gin.Param{Key: "id", Value: "c1"}}
	h.Approve(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestAPICollectionApprove_InvalidJSON(t *testing.T) {
	h, _ := newAPICollectionHandler()
	c, w := newGinContext("POST", "/api-collections/c1/approve", "not-json")
	c.Params = gin.Params{gin.Param{Key: "id", Value: "c1"}}
	h.Approve(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
