package vault

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newHealthServer returns an httptest server emulating Vault's /v1/sys/health
// endpoint with the given flat JSON body (sys/health does NOT use the
// {data:...} secret envelope).
func newHealthServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sys/health" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestClient(t *testing.T, addr string) *Client {
	t.Helper()
	t.Setenv("VAULT_ADDR", addr)
	t.Setenv("VAULT_TOKEN", "test-token")
	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestClient_IsAvailable_InitializedAndUnsealed(t *testing.T) {
	srv := newHealthServer(t, `{"initialized":true,"sealed":false,"standby":false,"version":"1.18.5"}`)
	c := newTestClient(t, srv.URL)

	if !c.IsAvailable(context.Background()) {
		t.Fatal("expected vault to be available (initialized + unsealed)")
	}
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestClient_IsAvailable_Sealed(t *testing.T) {
	srv := newHealthServer(t, `{"initialized":true,"sealed":true,"standby":false}`)
	c := newTestClient(t, srv.URL)

	if c.IsAvailable(context.Background()) {
		t.Fatal("expected sealed vault to be unavailable")
	}
}

func TestClient_IsAvailable_Uninitialized(t *testing.T) {
	srv := newHealthServer(t, `{"initialized":false,"sealed":true,"standby":false}`)
	c := newTestClient(t, srv.URL)

	if c.IsAvailable(context.Background()) {
		t.Fatal("expected uninitialized vault to be unavailable")
	}
}

func TestClient_IsAvailable_Unreachable(t *testing.T) {
	srv := newHealthServer(t, `{"initialized":true,"sealed":false}`)
	c := newTestClient(t, srv.URL)
	srv.Close() // simulate server down

	if c.IsAvailable(context.Background()) {
		t.Fatal("expected unreachable vault to be unavailable")
	}
}

func TestClient_IsAvailable_NilClient(t *testing.T) {
	var c *Client
	if c.IsAvailable(context.Background()) {
		t.Fatal("expected nil client to be unavailable")
	}
	if err := c.Ping(context.Background()); err == nil {
		t.Fatal("expected nil client Ping to fail")
	}
}
