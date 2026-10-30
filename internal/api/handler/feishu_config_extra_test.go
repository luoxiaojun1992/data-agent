package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/domain/feishu"
	feishusvc "github.com/luoxiaojun1992/data-agent/internal/service/feishu"
)

// ── List ─────────────────────────────────────────────────────────────

func TestFeishuList_Success(t *testing.T) {
	h, svc := newFeishuHandler()
	var gotUserID string
	var gotPage, gotPageSize int
	patches := gomonkey.ApplyMethodFunc(svc, "ListByUser",
		func(_ context.Context, userID string, page, pageSize int) ([]*feishu.Config, int64, error) {
			gotUserID, gotPage, gotPageSize = userID, page, pageSize
			return []*feishu.Config{{ID: "f1"}}, int64(1), nil
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("GET", "/im/feishu/configs?page=2&page_size=5", "")
	c.Set("user_id", "u1")
	h.List(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotUserID != "u1" || gotPage != 2 || gotPageSize != 5 {
		t.Errorf("ListByUser(%q,%d,%d), want (u1,2,5)", gotUserID, gotPage, gotPageSize)
	}
}

func TestFeishuList_Error(t *testing.T) {
	h, svc := newFeishuHandler()
	patches := gomonkey.ApplyMethodFunc(svc, "ListByUser",
		func(_ context.Context, userID string, page, pageSize int) ([]*feishu.Config, int64, error) {
			return nil, 0, errors.New("db down")
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("GET", "/im/feishu/configs", "")
	c.Set("user_id", "u1")
	h.List(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── Get ──────────────────────────────────────────────────────────────

func TestFeishuGet_OK(t *testing.T) {
	h, svc := newFeishuHandler()
	patches := gomonkey.ApplyMethodFunc(svc, "Get",
		func(_ context.Context, id string) (*feishu.Config, error) {
			return &feishu.Config{ID: id, Name: "bot"}, nil
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("GET", "/im/feishu/configs/f1", "")
	c.Params = gin.Params{gin.Param{Key: "id", Value: "f1"}}
	h.Get(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ── Update ───────────────────────────────────────────────────────────

func TestFeishuUpdate_BadJSON(t *testing.T) {
	h, _ := newFeishuHandler()
	c, w := newGinContext("PUT", "/im/feishu/configs/f1", `{bad`)
	c.Params = gin.Params{gin.Param{Key: "id", Value: "f1"}}
	h.Update(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestFeishuUpdate_ServiceError(t *testing.T) {
	h, svc := newFeishuHandler()
	patches := gomonkey.ApplyMethodFunc(svc, "Update",
		func(_ context.Context, id string, req feishusvc.UpdateConfigRequest) error {
			return errors.New("boom")
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("PUT", "/im/feishu/configs/f1", `{}`)
	c.Params = gin.Params{gin.Param{Key: "id", Value: "f1"}}
	h.Update(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── Delete ───────────────────────────────────────────────────────────

func TestFeishuDelete_OK(t *testing.T) {
	h, svc := newFeishuHandler()
	var gotID string
	patches := gomonkey.ApplyMethodFunc(svc, "Delete",
		func(_ context.Context, id string) error { gotID = id; return nil })
	t.Cleanup(patches.Reset)

	c, w := newGinContext("DELETE", "/im/feishu/configs/f1", "")
	c.Params = gin.Params{gin.Param{Key: "id", Value: "f1"}}
	h.Delete(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotID != "f1" {
		t.Errorf("Delete id = %q, want f1", gotID)
	}
}
