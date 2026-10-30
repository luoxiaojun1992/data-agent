package apicollection

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agiledragon/gomonkey/v2"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
)

// ── CreateUpload error branches ──────────────────────────────────────

func TestCreateUpload_RepoError(t *testing.T) {
	s, repo := newService()
	patches := gomonkey.ApplyMethodFunc(repo, "Create",
		func(_ context.Context, c *model.APICollection) error { return errors.New("db down") })
	t.Cleanup(patches.Reset)

	if _, err := s.CreateUpload(context.Background(), "u1", "n", "d", []byte(`{"openapi":"3.0.0"}`), "f1"); err == nil {
		t.Fatal("expected repo error")
	}
}

// ── Get repo error ───────────────────────────────────────────────────

func TestGet_RepoError(t *testing.T) {
	s, repo := newService()
	patches := gomonkey.ApplyMethodFunc(repo, "GetByID",
		func(_ context.Context, id string) (*model.APICollection, error) { return nil, errors.New("nf") })
	t.Cleanup(patches.Reset)

	if _, err := s.Get(context.Background(), "c1", ""); err == nil {
		t.Fatal("expected repo error")
	}
}

// ── GetAPISummary pagination edge cases ──────────────────────────────

func TestGetAPISummary_PageClampHigh(t *testing.T) {
	s, repo := newService()
	spec := json.RawMessage(`{"openapi":"3.0.0","paths":{"/a":{"get":{}}}}`)
	patches := gomonkey.ApplyMethodFunc(repo, "GetByID",
		func(_ context.Context, id string) (*model.APICollection, error) {
			return &model.APICollection{ID: id, Status: model.APICollectionApproved, OpenAPISpec: spec}, nil
		})
	t.Cleanup(patches.Reset)

	res, err := s.GetAPISummary(context.Background(), "c1", 150, 10)
	if err != nil {
		t.Fatalf("GetAPISummary() error = %v", err)
	}
	if res.Page != 100 {
		t.Errorf("Page = %d, want 100 (clamped)", res.Page)
	}
}

func TestGetAPISummary_PageBeyondTotal(t *testing.T) {
	s, repo := newService()
	spec := json.RawMessage(`{"openapi":"3.0.0","paths":{"/a":{"get":{}}}}`)
	patches := gomonkey.ApplyMethodFunc(repo, "GetByID",
		func(_ context.Context, id string) (*model.APICollection, error) {
			return &model.APICollection{ID: id, Status: model.APICollectionApproved, OpenAPISpec: spec}, nil
		})
	t.Cleanup(patches.Reset)

	res, err := s.GetAPISummary(context.Background(), "c1", 5, 10)
	if err != nil {
		t.Fatalf("GetAPISummary() error = %v", err)
	}
	// start=(5-1)*10=40 > total=1 → clamped to total, so paths empty, total preserved.
	if res.Total != 1 {
		t.Errorf("Total = %d, want 1", res.Total)
	}
	if len(res.Paths) != 0 {
		t.Errorf("Paths = %d, want 0 (page beyond total)", len(res.Paths))
	}
}

// ── GetAPIMethod not-approved / missing-paths ────────────────────────

func TestGetAPIMethod_NotApproved(t *testing.T) {
	s, repo := newService()
	patches := gomonkey.ApplyMethodFunc(repo, "GetByID",
		func(_ context.Context, id string) (*model.APICollection, error) {
			return &model.APICollection{ID: id, Status: model.APICollectionPending}, nil
		})
	t.Cleanup(patches.Reset)

	if _, err := s.GetAPIMethod(context.Background(), "c1", "/a", "get"); err == nil {
		t.Fatal("expected 'collection not approved' error")
	}
}

func TestGetAPIMethod_NoPaths(t *testing.T) {
	s, repo := newService()
	spec := json.RawMessage(`{"openapi":"3.0.0"}`)
	patches := gomonkey.ApplyMethodFunc(repo, "GetByID",
		func(_ context.Context, id string) (*model.APICollection, error) {
			return &model.APICollection{ID: id, Status: model.APICollectionApproved, OpenAPISpec: spec}, nil
		})
	t.Cleanup(patches.Reset)

	if _, err := s.GetAPIMethod(context.Background(), "c1", "/a", "get"); err == nil {
		t.Fatal("expected 'path not found' error when spec has no paths")
	}
}

// ── CallAPI body / no-servers / http-error branches ──────────────────

func TestCallAPI_WithBody(t *testing.T) {
	var gotContentType, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	s, repo := newService()
	spec := json.RawMessage(`{"openapi":"3.0.0","servers":[{"url":"` + srv.URL + `"}],"paths":{"/a":{"post":{}}}}`)
	patches := gomonkey.ApplyMethodFunc(repo, "GetByID",
		func(_ context.Context, id string) (*model.APICollection, error) {
			return &model.APICollection{ID: id, Status: model.APICollectionApproved, OpenAPISpec: spec}, nil
		})
	t.Cleanup(patches.Reset)

	body := map[string]any{"a": 1}
	_, err := s.CallAPI(context.Background(), "c1", "/a", "post", nil, body, nil)
	if err != nil {
		t.Fatalf("CallAPI() error = %v", err)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if !strings.Contains(gotBody, `"a":1`) {
		t.Errorf("body = %q, want serialized JSON", gotBody)
	}
}

func TestCallAPI_NoServers(t *testing.T) {
	s, repo := newService()
	spec := json.RawMessage(`{"openapi":"3.0.0","paths":{"/a":{"get":{}}}}`)
	patches := gomonkey.ApplyMethodFunc(repo, "GetByID",
		func(_ context.Context, id string) (*model.APICollection, error) {
			return &model.APICollection{ID: id, Status: model.APICollectionApproved, OpenAPISpec: spec}, nil
		})
	t.Cleanup(patches.Reset)

	// No servers → baseURL empty → fullURL is a relative path → NewRequest errors.
	if _, err := s.CallAPI(context.Background(), "c1", "/a", "get", nil, nil, nil); err == nil {
		t.Fatal("expected error when no servers defined (relative URL)")
	}
}

func TestCallAPI_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // connection refused afterwards

	s, repo := newService()
	spec := json.RawMessage(`{"openapi":"3.0.0","servers":[{"url":"` + url + `"}],"paths":{"/a":{"get":{}}}}`)
	patches := gomonkey.ApplyMethodFunc(repo, "GetByID",
		func(_ context.Context, id string) (*model.APICollection, error) {
			return &model.APICollection{ID: id, Status: model.APICollectionApproved, OpenAPISpec: spec}, nil
		})
	t.Cleanup(patches.Reset)

	if _, err := s.CallAPI(context.Background(), "c1", "/a", "get", nil, nil, nil); err == nil {
		t.Fatal("expected HTTP client error against closed server")
	}
}
