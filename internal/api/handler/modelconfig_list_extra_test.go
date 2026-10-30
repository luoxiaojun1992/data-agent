package handler

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

// newModelListHandler builds a handler over a Provider whose repository returns
// the given entries (and the given error) for both List and Search, so the
// ListEmbedding / ListLLM handler branches (paginated vs dropdown search, and
// their error paths) can be exercised directly.
func newModelListHandler(t *testing.T, entries []modelcfg.ModelEntry, err error) *ModelConfigHandler {
	t.Helper()
	repo := mockrepo.NewModelConfigRepository(t)
	defRepo := mockrepo.NewModelDefaultRepository(t)

	repo.On("List", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(entries, int64(len(entries)), err).Maybe()
	repo.On("Search", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(entries, int64(len(entries)), err).Maybe()

	defRepo.On("List", mock.Anything).Return(nil, nil).Maybe()
	defRepo.On("Get", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("no default")).Maybe()

	p := modelcfg.NewProvider(repo, defRepo, nil)
	return NewModelConfigHandler(nil, p)
}

// ── ListEmbedding ──

func TestModelConfig_ListEmbedding_Paginated(t *testing.T) {
	h := newModelListHandler(t, []modelcfg.ModelEntry{
		{ID: "e1", Name: "E1", Type: modelcfg.ModelTypeEmbedding},
		{ID: "e2", Name: "E2", Type: modelcfg.ModelTypeEmbedding},
	}, nil)

	c, w := ginReq("GET", "/models/embedding?page=1&page_size=10", nil)
	h.ListEmbedding(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !containsJSONKey(w.Body.String(), `"total":2`) {
		t.Errorf("expected total=2, got: %s", w.Body.String())
	}
}

func TestModelConfig_ListEmbedding_Search(t *testing.T) {
	h := newModelListHandler(t, []modelcfg.ModelEntry{
		{ID: "e1", Name: "E1", Type: modelcfg.ModelTypeEmbedding},
	}, nil)

	c, w := ginReq("GET", "/models/embedding?q=emb", nil)
	h.ListEmbedding(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestModelConfig_ListEmbedding_Error(t *testing.T) {
	h := newModelListHandler(t, nil, fmt.Errorf("db down"))

	c, w := ginReq("GET", "/models/embedding", nil)
	h.ListEmbedding(c)

	if w.Code != 500 {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestModelConfig_ListEmbedding_NoProvider(t *testing.T) {
	h := NewModelConfigHandler(nil, nil)
	c, w := ginReq("GET", "/models/embedding", nil)
	h.ListEmbedding(c)
	if w.Code != 503 {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

// ── ListLLM dropdown search (SPEC-074) ──

func TestModelConfig_ListLLM_Search(t *testing.T) {
	h := newModelListHandler(t, []modelcfg.ModelEntry{
		{ID: "m1", Name: "M1", Type: modelcfg.ModelTypeLLM},
	}, nil)

	c, w := ginReq("GET", "/models/list?q=deep", nil)
	h.ListLLM(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestModelConfig_ListLLM_Search_Error(t *testing.T) {
	h := newModelListHandler(t, nil, fmt.Errorf("db down"))

	c, w := ginReq("GET", "/models/list?q=deep", nil)
	h.ListLLM(c)

	if w.Code != 500 {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestModelConfig_ListLLM_Paginated_Error(t *testing.T) {
	h := newModelListHandler(t, nil, fmt.Errorf("db down"))

	c, w := ginReq("GET", "/models/list?page=1", nil)
	h.ListLLM(c)

	if w.Code != 500 {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── getPaginated (Get?page=) error path ──

func TestModelConfig_Get_PaginatedError(t *testing.T) {
	h := newModelListHandler(t, nil, fmt.Errorf("db down"))

	c, w := ginReq("GET", "/models?page=1", nil)
	h.Get(c)

	if w.Code != 500 {
		t.Errorf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

// containsJSONKey checks that the raw response body contains the given
// JSON key/value fragment.
func containsJSONKey(body, fragment string) bool {
	for i := 0; i+len(fragment) <= len(body); i++ {
		if body[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
