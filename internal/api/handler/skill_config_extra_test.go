package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/domain/skill"
)

func TestSkillGet_OK(t *testing.T) {
	h, svc := newSkillHandler()
	patches := gomonkey.ApplyMethodFunc(svc, "Get",
		func(_ context.Context, name string) (*skill.SkillConfig, error) {
			return &skill.SkillConfig{Name: name, Enabled: true}, nil
		})
	t.Cleanup(patches.Reset)

	c, w := newGinContext("GET", "/admin/skills/sql_executor", "")
	c.Params = gin.Params{gin.Param{Key: "name", Value: "sql_executor"}}
	h.Get(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSkillUpsert_ServiceError(t *testing.T) {
	h, svc := newSkillHandler()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(svc, "Get",
		func(_ context.Context, name string) (*skill.SkillConfig, error) {
			return &skill.SkillConfig{Name: name}, nil
		})
	patches.ApplyMethodFunc(svc, "Upsert",
		func(_ context.Context, cfg skill.SkillConfig) error { return errors.New("boom") })

	c, w := newGinContext("PUT", "/admin/skills/x", `{"enabled":true}`)
	c.Params = gin.Params{gin.Param{Key: "name", Value: "x"}}
	h.Upsert(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
