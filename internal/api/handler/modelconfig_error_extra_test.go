package handler

import (
	"context"
	"fmt"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

// SetDefault embedding branch (LLM branch + not-found + no-provider already
// covered in modelconfig_crud_test.go).

func TestModelConfig_SetDefault_Embedding(t *testing.T) {
	h := newModelTestHandler(t, []modelcfg.ModelEntry{
		{ID: "e1", Name: "E1", Type: modelcfg.ModelTypeEmbedding},
	})
	c, w := ginReq("PATCH", "/models/e1/default", nil)
	c.Params = gin.Params{{Key: "id", Value: "e1"}}
	h.SetDefault(c)
	if w.Code != 200 {
		t.Fatalf("expected 200 for embedding default, got %d: %s", w.Code, w.Body.String())
	}
}

func TestModelConfig_SetDefault_EmbeddingError(t *testing.T) {
	repo := mockrepo.NewModelConfigRepository(t)
	defRepo := mockrepo.NewModelDefaultRepository(t)
	repo.On("List", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]modelcfg.ModelEntry{{ID: "e1", Name: "E1", Type: modelcfg.ModelTypeEmbedding}}, int64(1), nil)
	repo.On("Get", mock.Anything, "e1").
		Return(&modelcfg.ModelEntry{ID: "e1", Name: "E1", Type: modelcfg.ModelTypeEmbedding}, nil)
	defRepo.On("List", mock.Anything).Return(nil, nil).Maybe()
	defRepo.On("Set", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("db down"))

	p := modelcfg.NewProvider(repo, defRepo, nil)
	h := NewModelConfigHandler(nil, p)

	c, w := ginReq("PATCH", "/models/e1/default", nil)
	c.Params = gin.Params{{Key: "id", Value: "e1"}}
	h.SetDefault(c)
	if w.Code != 400 {
		t.Fatalf("expected 400 when SetDefaultEmbedding fails, got %d", w.Code)
	}
}

func TestModelConfig_SetDefault_ModelError(t *testing.T) {
	repo := mockrepo.NewModelConfigRepository(t)
	defRepo := mockrepo.NewModelDefaultRepository(t)
	repo.On("List", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]modelcfg.ModelEntry{{ID: "m1", Name: "M1", Type: modelcfg.ModelTypeLLM}}, int64(1), nil)
	defRepo.On("List", mock.Anything).Return(nil, nil).Maybe()
	defRepo.On("Set", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("db down"))

	p := modelcfg.NewProvider(repo, defRepo, nil)
	h := NewModelConfigHandler(nil, p)

	c, w := ginReq("PATCH", "/models/m1/default", nil)
	c.Params = gin.Params{{Key: "id", Value: "m1"}}
	h.SetDefault(c)
	if w.Code != 400 {
		t.Fatalf("expected 400 when SetDefaultModel fails, got %d", w.Code)
	}
}

// DeleteModel error branch (success/idempotent/no-provider already covered).

func TestModelConfig_DeleteModel_Error(t *testing.T) {
	repo := mockrepo.NewModelConfigRepository(t)
	defRepo := mockrepo.NewModelDefaultRepository(t)
	repo.On("Delete", mock.Anything, "m1").Return(fmt.Errorf("db down"))

	p := modelcfg.NewProvider(repo, defRepo, nil)
	h := NewModelConfigHandler(nil, p)

	c, w := ginReq("DELETE", "/models/m1", nil)
	c.Params = gin.Params{{Key: "id", Value: "m1"}}
	h.DeleteModel(c)
	if w.Code != 500 {
		t.Fatalf("expected 500 when DeleteModel fails, got %d", w.Code)
	}
}

// getRaw's error branch is defensive: the concrete Provider.GetRawModelConfig
// currently never returns an error, but the handler must still map one to 500
// if that ever changes.
func TestModelConfig_GetRaw_Error(t *testing.T) {
	h := newModelTestHandler(t, nil)
	patches := gomonkey.ApplyMethodFunc(&modelcfg.Provider{}, "GetRawModelConfig",
		func(_ context.Context) (map[string]any, error) {
			return nil, fmt.Errorf("db down")
		})
	defer patches.Reset()

	c, w := ginReq("GET", "/models", nil)
	h.Get(c)
	if w.Code != 500 {
		t.Fatalf("expected 500 when GetRawModelConfig fails, got %d", w.Code)
	}
}
