package hermes

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeServer returns a real HTTP test server that acts as the hermes backend.
// Using a real server eliminates the need for gomonkey which fails on Linux + -race.
func fakeServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestNewService(t *testing.T) {
	s := NewService("http://localhost:8080")
	if s == nil {
		t.Fatal("NewService() should not return nil")
	}
	if s.hermesURL != "http://localhost:8080" {
		t.Errorf("hermesURL = %q, want %q", s.hermesURL, "http://localhost:8080")
	}
}

func TestNewService_EmptyURL(t *testing.T) {
	s := NewService("")
	if s == nil {
		t.Fatal("NewService() should not return nil even with empty URL")
	}
}

func TestNewService_ClientTimeout(t *testing.T) {
	s := NewService("http://example.com")
	if s.client == nil {
		t.Fatal("client should not be nil")
	}
	if s.client.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", s.client.Timeout)
	}
}

func TestNewService_HermesURL(t *testing.T) {
	url := "https://hermes.example.com/api"
	s := NewService(url)
	if s.hermesURL != url {
		t.Errorf("hermesURL = %q, want %q", s.hermesURL, url)
	}
}

func TestNewService_InvalidURL(t *testing.T) {
	s := NewService("not-a-valid-url")
	if s == nil {
		t.Fatal("NewService should not return nil for invalid URL")
	}
}

func TestProxy_Success(t *testing.T) {
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "hello from hermes")
	})
	s := NewService(srv.URL)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/explore", nil)
	s.Proxy(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	if w.Body.String() != "hello from hermes" {
		t.Errorf("body: got %q, want %q", w.Body.String(), "hello from hermes")
	}
}

func TestProxy_Error(t *testing.T) {
	// Point to an unreachable address to force client.Do to fail.
	s := NewService("http://127.0.0.1:1") // port 1 is reserved, no service

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/explore", nil)
	s.Proxy(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d, want 503", w.Code)
	}
}

func TestProxy_StatusCode(t *testing.T) {
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"not found"}`)
	})
	s := NewService(srv.URL)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/nonexistent", nil)
	s.Proxy(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", w.Code)
	}
	if w.Body.String() != `{"error":"not found"}` {
		t.Errorf("body: got %q, want %q", w.Body.String(), `{"error":"not found"}`)
	}
}

func TestProxy_HeadersForwarded(t *testing.T) {
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "custom-value")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{}`)
	})
	s := NewService(srv.URL)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/test", nil)
	s.Proxy(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	if w.Header().Get("X-Custom-Header") != "custom-value" {
		t.Errorf("X-Custom-Header: got %q, want %q", w.Header().Get("X-Custom-Header"), "custom-value")
	}
}

func TestProxy_EmptyBody(t *testing.T) {
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	s := NewService(srv.URL)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/empty", nil)
	s.Proxy(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("status: got %d, want 204", w.Code)
	}
	if w.Body.String() != "" {
		t.Errorf("body: got %q, want empty", w.Body.String())
	}
}

func TestProxy_InternalError(t *testing.T) {
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "internal error")
	})
	s := NewService(srv.URL)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/broken", nil)
	s.Proxy(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want 500", w.Code)
	}
}

func TestProxy_NewRequestError(t *testing.T) {
	// An invalid URL (control character) causes http.NewRequest to fail.
	s := NewService("http://\x7f/")
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/test", nil)
	s.Proxy(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want 500", w.Code)
	}
	if !strings.Contains(w.Body.String(), "proxy error") {
		t.Errorf("body: got %q, want proxy error", w.Body.String())
	}
}

func TestProxy_IOCopyError(t *testing.T) {
	// Server returns a chunked-encoded body that becomes invalid mid-stream.
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		// Hijack the connection and write an invalid chunked body
		// so io.Copy on the client side fails.
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Skip("ResponseWriter does not support Hijacker")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		// Write a malformed chunked response.
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nTransfer-Encoding: chunked\r\n\r\n"))
		_, _ = conn.Write([]byte("5\r\nhello\r")) // truncated, no terminating CRLF
	})
	s := NewService(srv.URL)
	// Force a short timeout so the proxy returns quickly even on slow failure.
	s.client.Timeout = 2 * time.Second

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/test", nil)
	s.Proxy(w, r)
	// Status code depends on whether io.Copy error happens before/after status flush.
	// We just verify the proxy handled the failure without panicking.
	if w.Code == 0 {
		t.Errorf("status should be set, got 0")
	}
}

func TestProxy_PostMethod(t *testing.T) {
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("forwarded method: got %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id": 1}`)
	})
	s := NewService(srv.URL)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/create", strings.NewReader(`{"name":"test"}`))
	s.Proxy(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("status: got %d, want 201", w.Code)
	}
}

func TestProxy_RequestHeadersForwarded(t *testing.T) {
	var capturedReq *http.Request
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		w.WriteHeader(http.StatusOK)
	})
	s := NewService(srv.URL)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/test", nil)
	r.Header.Set("Authorization", "Bearer token123")
	r.Header.Set("X-Custom-Header", "value")
	s.Proxy(w, r)
	if capturedReq == nil {
		t.Fatal("server did not receive request")
	}
	if capturedReq.Header.Get("Authorization") != "Bearer token123" {
		t.Errorf("Authorization: got %q", capturedReq.Header.Get("Authorization"))
	}
	if capturedReq.Header.Get("X-Custom-Header") != "value" {
		t.Errorf("X-Custom-Header: got %q", capturedReq.Header.Get("X-Custom-Header"))
	}
}

func TestProxy_MultipleResponseHeaders(t *testing.T) {
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Set-Cookie", "session=abc")
		w.Header().Add("Set-Cookie", "user=xyz")
		w.WriteHeader(http.StatusOK)
	})
	s := NewService(srv.URL)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/test", nil)
	s.Proxy(w, r)
	cookies := w.Header()["Set-Cookie"]
	if len(cookies) != 2 {
		t.Fatalf("Set-Cookie count: got %d, want 2", len(cookies))
	}
	if cookies[0] != "session=abc" {
		t.Errorf("Set-Cookie[0]: got %q", cookies[0])
	}
}

func TestProxy_EmptyResponseHeaders(t *testing.T) {
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "no headers")
	})
	s := NewService(srv.URL)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/test", nil)
	s.Proxy(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
}

func TestProxy_Timeout(t *testing.T) {
	// Server never responds; client times out.
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
	})
	defer srv.Close()
	s := NewService(srv.URL)
	s.client.Timeout = 100 * time.Millisecond

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/slow", nil)
	s.Proxy(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d, want 503", w.Code)
	}
}

func TestProxy_NewService_NilClientCheck(t *testing.T) {
	s := NewService("http://test:8080")
	if s.client == nil {
		t.Fatal("client should not be nil")
	}
	if s.client.Timeout == 0 {
		t.Error("client timeout should be set")
	}
}

// Keep references to imports that may otherwise be unused after refactor.
var (
	_ = errors.New
	_ = io.EOF
	_ = httptest.NewRecorder
)
