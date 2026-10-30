package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/domain/skill"
	skillsvc "github.com/luoxiaojun1992/data-agent/internal/service/skill"
)

func newSkillHandler() (*SkillConfigHandler, *skillsvc.ConfigService) {
	svc := &skillsvc.ConfigService{}
	return NewSkillConfigHandler(svc), svc
}

func TestSkillList_Success(t *testing.T) {
	h, svc := newSkillHandler()
	patches := gomonkey.ApplyMethodFunc(svc, "List",
		func(_ context.Context, page, pageSize int) ([]skill.SkillConfig, int, error) {
			return []skill.SkillConfig{{Name: "sql_executor"}}, 1, nil
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("GET", "/admin/skills", "")
	h.List(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSkillList_Error(t *testing.T) {
	h, svc := newSkillHandler()
	patches := gomonkey.ApplyMethodFunc(svc, "List",
		func(_ context.Context, page, pageSize int) ([]skill.SkillConfig, int, error) {
			return nil, 0, errors.New("db down")
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("GET", "/admin/skills", "")
	h.List(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestSkillGet_NotFound(t *testing.T) {
	h, svc := newSkillHandler()
	patches := gomonkey.ApplyMethodFunc(svc, "Get",
		func(_ context.Context, name string) (*skill.SkillConfig, error) {
			return nil, errors.New("unknown skill")
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("GET", "/admin/skills/nope", "")
	c.Params = gin.Params{gin.Param{Key: "name", Value: "nope"}}
	h.Get(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSkillUpsert_InvalidJSON(t *testing.T) {
	h, _ := newSkillHandler()
	c, w := newGinContext("PUT", "/admin/skills/x", `{bad`)
	c.Params = gin.Params{gin.Param{Key: "name", Value: "x"}}
	h.Upsert(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSkillUpsert_UnknownSkill(t *testing.T) {
	h, svc := newSkillHandler()
	patches := gomonkey.ApplyMethodFunc(svc, "Get",
		func(_ context.Context, name string) (*skill.SkillConfig, error) {
			return nil, errors.New("unknown skill")
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("PUT", "/admin/skills/nope", `{"enabled":true}`)
	c.Params = gin.Params{gin.Param{Key: "name", Value: "nope"}}
	h.Upsert(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSkillUpsert_MutatesAndSaves(t *testing.T) {
	h, svc := newSkillHandler()
	var saved *skill.SkillConfig
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(svc, "Get",
		func(_ context.Context, name string) (*skill.SkillConfig, error) {
			return &skill.SkillConfig{Name: name, Enabled: false}, nil
		})
	patches.ApplyMethodFunc(svc, "Upsert",
		func(_ context.Context, cfg skill.SkillConfig) error {
			saved = &cfg
			return nil
		})

	c, w := newGinContext("PUT", "/admin/skills/file_delete",
		`{"enabled":true,"config_json":"{}","requires_approval":true}`)
	c.Params = gin.Params{gin.Param{Key: "name", Value: "file_delete"}}
	h.Upsert(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if saved == nil {
		t.Fatal("Upsert was not called")
	}
	if !saved.Enabled || !saved.RequiresApproval {
		t.Errorf("saved skill = %+v, want enabled + requires_approval", saved)
	}
}
