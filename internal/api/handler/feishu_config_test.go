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

func newFeishuHandler() (*FeishuConfigHandler, *feishusvc.ConfigService) {
	svc := &feishusvc.ConfigService{}
	return NewFeishuConfigHandler(svc), svc
}

func TestFeishuCreate_MissingRequiredFields(t *testing.T) {
	h, _ := newFeishuHandler()
	c, w := newGinContext("POST", "/im/feishu/configs", `{}`)
	c.Set("user_id", "u1")
	h.Create(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing required fields, got %d", w.Code)
	}
}

func TestFeishuCreate_Success(t *testing.T) {
	h, svc := newFeishuHandler()
	var gotUserID, gotName, gotAppID, gotSecret, gotModelID string
	patches := gomonkey.ApplyMethodFunc(svc, "Create",
		func(_ context.Context, userID, name, appID, appSecret, modelID string) (*feishu.Config, error) {
			gotUserID, gotName, gotAppID, gotSecret, gotModelID = userID, name, appID, appSecret, modelID
			return &feishu.Config{ID: "f1"}, nil
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("POST", "/im/feishu/configs",
		`{"name":"bot","app_id":"app_1","app_secret":"s3cret","model_id":"m1"}`)
	c.Set("user_id", "u1")
	h.Create(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if gotUserID != "u1" || gotName != "bot" || gotAppID != "app_1" || gotSecret != "s3cret" || gotModelID != "m1" {
		t.Errorf("Create args = (%q,%q,%q,%q,%q)", gotUserID, gotName, gotAppID, gotSecret, gotModelID)
	}
}

func TestFeishuCreate_ServiceError(t *testing.T) {
	h, svc := newFeishuHandler()
	patches := gomonkey.ApplyMethodFunc(svc, "Create",
		func(_ context.Context, userID, name, appID, appSecret, modelID string) (*feishu.Config, error) {
			return nil, errors.New("boom")
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("POST", "/im/feishu/configs",
		`{"name":"bot","app_id":"app_1","app_secret":"s3cret"}`)
	c.Set("user_id", "u1")
	h.Create(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestFeishuGet_NotFound(t *testing.T) {
	h, svc := newFeishuHandler()
	patches := gomonkey.ApplyMethodFunc(svc, "Get",
		func(_ context.Context, id string) (*feishu.Config, error) {
			return nil, errors.New("not found")
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("GET", "/im/feishu/configs/f1", "")
	c.Params = gin.Params{gin.Param{Key: "id", Value: "f1"}}
	h.Get(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestFeishuUpdate_MapsFields(t *testing.T) {
	h, svc := newFeishuHandler()
	var gotID string
	var gotReq feishusvc.UpdateConfigRequest
	patches := gomonkey.ApplyMethodFunc(svc, "Update",
		func(_ context.Context, id string, req feishusvc.UpdateConfigRequest) error {
			gotID, gotReq = id, req
			return nil
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("PUT", "/im/feishu/configs/f1",
		`{"name":"new","app_id":"app_2","app_secret":"s2","enabled":false}`)
	c.Params = gin.Params{gin.Param{Key: "id", Value: "f1"}}
	h.Update(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotID != "f1" {
		t.Errorf("Update id = %q, want f1", gotID)
	}
	if gotReq.Name != "new" || gotReq.AppID != "app_2" || gotReq.AppSecret != "s2" {
		t.Errorf("Update req = %+v", gotReq)
	}
	if gotReq.Enabled == nil || *gotReq.Enabled != false {
		t.Errorf("Enabled should be a false pointer, got %+v", gotReq.Enabled)
	}
}

func TestFeishuDelete_ServiceError(t *testing.T) {
	h, svc := newFeishuHandler()
	patches := gomonkey.ApplyMethodFunc(svc, "Delete",
		func(_ context.Context, id string) error { return errors.New("boom") })
	t.Cleanup(patches.Reset)

	c, w := newGinContext("DELETE", "/im/feishu/configs/f1", "")
	c.Params = gin.Params{gin.Param{Key: "id", Value: "f1"}}
	h.Delete(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}
