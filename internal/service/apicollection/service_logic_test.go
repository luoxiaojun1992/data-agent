package apicollection

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agiledragon/gomonkey/v2"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
)

func newService() (*Service, *mongo.APICollectionRepo) {
	repo := &mongo.APICollectionRepo{}
	return NewService(repo), repo
}

func TestCreateUpload_Success(t *testing.T) {
	s, repo := newService()
	var created *model.APICollection
	patches := gomonkey.ApplyMethodFunc(repo, "Create",
		func(_ context.Context, c *model.APICollection) error {
			created = c
			return nil
		})
	t.Cleanup(patches.Reset)

	spec := `{"openapi":"3.0.0","paths":{"/a":{},"/b":{}}}`
	coll, err := s.CreateUpload(context.Background(), "u1", "n", "d", []byte(spec), "f1")
	if err != nil {
		t.Fatalf("CreateUpload() error = %v", err)
	}
	if coll == nil || created == nil {
		t.Fatal("CreateUpload() returned nil")
	}
	if coll.ID == "" {
		t.Error("ID should be generated")
	}
	if coll.UserID != "u1" || coll.Name != "n" || coll.Description != "d" || coll.FileID != "f1" {
		t.Errorf("unexpected fields: %+v", coll)
	}
	if coll.Status != model.APICollectionPending {
		t.Errorf("Status = %q, want pending", coll.Status)
	}
	if coll.APICount != 2 {
		t.Errorf("APICount = %d, want 2", coll.APICount)
	}
}

func TestGet_OwnershipMismatch(t *testing.T) {
	s, repo := newService()
	patches := gomonkey.ApplyMethodFunc(repo, "GetByID",
		func(_ context.Context, id string) (*model.APICollection, error) {
			return &model.APICollection{ID: id, UserID: "other"}, nil
		})
	t.Cleanup(patches.Reset)

	if _, err := s.Get(context.Background(), "c1", "me"); err == nil {
		t.Fatal("expected 'not found' when owner mismatch")
	}
}

func TestGet_NoOwnerFilter(t *testing.T) {
	s, repo := newService()
	patches := gomonkey.ApplyMethodFunc(repo, "GetByID",
		func(_ context.Context, id string) (*model.APICollection, error) {
			return &model.APICollection{ID: id, UserID: "any"}, nil
		})
	t.Cleanup(patches.Reset)

	// ownerID empty → no ownership check
	coll, err := s.Get(context.Background(), "c1", "")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if coll.ID != "c1" {
		t.Errorf("ID = %q", coll.ID)
	}
}

func TestGetAPISummary_NotApproved(t *testing.T) {
	s, repo := newService()
	patches := gomonkey.ApplyMethodFunc(repo, "GetByID",
		func(_ context.Context, id string) (*model.APICollection, error) {
			return &model.APICollection{ID: id, Status: model.APICollectionPending}, nil
		})
	t.Cleanup(patches.Reset)

	if _, err := s.GetAPISummary(context.Background(), "c1", 1, 10); err == nil {
		t.Fatal("expected 'collection not approved' error")
	}
}

func TestGetAPISummary_Success(t *testing.T) {
	s, repo := newService()
	spec := json.RawMessage(`{
		"openapi":"3.0.0",
		"paths":{
			"/users":{"get":{"summary":"List users"},"parameters":[]},
			"/orders/{id}":{"post":{"summary":"Create order"},"servers":[]}
		}
	}`)
	patches := gomonkey.ApplyMethodFunc(repo, "GetByID",
		func(_ context.Context, id string) (*model.APICollection, error) {
			return &model.APICollection{ID: id, Name: "svc", Status: model.APICollectionApproved, OpenAPISpec: spec}, nil
		})
	t.Cleanup(patches.Reset)

	res, err := s.GetAPISummary(context.Background(), "c1", 1, 10)
	if err != nil {
		t.Fatalf("GetAPISummary() error = %v", err)
	}
	if res.Total != 2 {
		t.Fatalf("Total = %d, want 2", res.Total)
	}
	if len(res.Paths) != 2 {
		t.Fatalf("Paths = %d, want 2", len(res.Paths))
	}
	if res.Name != "svc" {
		t.Errorf("Name = %q, want svc", res.Name)
	}
	// method keys like "parameters"/"servers" are filtered out, not emitted.
	for _, p := range res.Paths {
		if p.Method == "parameters" || p.Method == "servers" {
			t.Errorf("filtered key %q leaked into paths", p.Method)
		}
	}
}

func TestGetAPISummary_PaginationClamp(t *testing.T) {
	s, repo := newService()
	spec := json.RawMessage(`{"openapi":"3.0.0","paths":{"/a":{"get":{}}}}`)
	patches := gomonkey.ApplyMethodFunc(repo, "GetByID",
		func(_ context.Context, id string) (*model.APICollection, error) {
			return &model.APICollection{ID: id, Status: model.APICollectionApproved, OpenAPISpec: spec}, nil
		})
	t.Cleanup(patches.Reset)

	res, err := s.GetAPISummary(context.Background(), "c1", 0, 999)
	if err != nil {
		t.Fatalf("GetAPISummary() error = %v", err)
	}
	// page 0 → 1, pageSize 999 → 10
	if res.Page != 1 || res.PageSize != 10 {
		t.Errorf("Page=%d PageSize=%d, want 1/10", res.Page, res.PageSize)
	}
}

func TestGetAPIMethod_NotFound(t *testing.T) {
	s, repo := newService()
	spec := json.RawMessage(`{"openapi":"3.0.0","paths":{"/a":{"get":{"summary":"x"}}}}`)
	patches := gomonkey.ApplyMethodFunc(repo, "GetByID",
		func(_ context.Context, id string) (*model.APICollection, error) {
			return &model.APICollection{ID: id, Status: model.APICollectionApproved, OpenAPISpec: spec}, nil
		})
	t.Cleanup(patches.Reset)

	if _, err := s.GetAPIMethod(context.Background(), "c1", "/nope", "get"); err == nil {
		t.Fatal("expected 'path not found' error")
	}
	if _, err := s.GetAPIMethod(context.Background(), "c1", "/a", "post"); err == nil {
		t.Fatal("expected 'method not found' error")
	}
}

func TestGetAPIMethod_Success(t *testing.T) {
	s, repo := newService()
	spec := json.RawMessage(`{
		"openapi":"3.0.0",
		"paths":{"/a":{"get":{"summary":"s","description":"d","parameters":[1],"requestBody":{"x":1},"responses":{"200":{}}}}}
	}`)
	patches := gomonkey.ApplyMethodFunc(repo, "GetByID",
		func(_ context.Context, id string) (*model.APICollection, error) {
			return &model.APICollection{ID: id, Status: model.APICollectionApproved, OpenAPISpec: spec}, nil
		})
	t.Cleanup(patches.Reset)

	d, err := s.GetAPIMethod(context.Background(), "c1", "/a", "get")
	if err != nil {
		t.Fatalf("GetAPIMethod() error = %v", err)
	}
	if d.Path != "/a" || d.Method != "get" || d.Summary != "s" || d.Description != "d" {
		t.Errorf("unexpected detail: %+v", d)
	}
	if d.Parameters == nil || d.RequestBody == nil || d.Responses == nil {
		t.Error("parameters/requestBody/responses should be populated")
	}
}

func TestCallAPI_Success(t *testing.T) {
	var gotPath, gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotHeader = r.Header.Get("X-Key")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	s, repo := newService()
	spec := json.RawMessage(`{
		"openapi":"3.0.0",
		"servers":[{"url":"` + srv.URL + `"}],
		"paths":{"/api/{id}":{"get":{}}}
	}`)
	patches := gomonkey.ApplyMethodFunc(repo, "GetByID",
		func(_ context.Context, id string) (*model.APICollection, error) {
			return &model.APICollection{ID: id, Status: model.APICollectionApproved, OpenAPISpec: spec}, nil
		})
	t.Cleanup(patches.Reset)

	res, err := s.CallAPI(context.Background(), "c1", "/api/{id}", "get",
		map[string]string{"id": "123"}, nil, map[string]string{"X-Key": "v"})
	if err != nil {
		t.Fatalf("CallAPI() error = %v", err)
	}
	if res.Status != http.StatusOK {
		t.Errorf("Status = %d, want 200", res.Status)
	}
	if gotPath != "/api/123" {
		t.Errorf("server saw path %q, want /api/123", gotPath)
	}
	if gotHeader != "v" {
		t.Errorf("server saw header X-Key=%q, want v", gotHeader)
	}
}

func TestCallAPI_NotApproved(t *testing.T) {
	s, repo := newService()
	patches := gomonkey.ApplyMethodFunc(repo, "GetByID",
		func(_ context.Context, id string) (*model.APICollection, error) {
			return &model.APICollection{ID: id, Status: model.APICollectionPending}, nil
		})
	t.Cleanup(patches.Reset)

	if _, err := s.CallAPI(context.Background(), "c1", "/a", "get", nil, nil, nil); err == nil {
		t.Fatal("expected 'collection not approved' error")
	}
}
