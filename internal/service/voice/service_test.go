package voice

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestService builds a Service wired to a fake sidecar that echoes a fixed
// transcription. Useful for exercising the full chunk→transcribe path.
func newTestService(t *testing.T, text string) (*Service, *Manager) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"` + text + `"}`))
	}))
	t.Cleanup(srv.Close)

	client := NewClient(ClientConfig{Endpoint: srv.URL})
	manager := NewManager(ManagerConfig{})
	return NewService(client, manager), manager
}

func TestService_FinishHappyPath(t *testing.T) {
	svc, _ := newTestService(t, "你好世界")

	id := svc.Start(context.Background(), "u1", "zh")
	if _, err := svc.Chunk(context.Background(), id, "u1", 0, []byte("pcm")); err != nil {
		t.Fatalf("chunk: %v", err)
	}
	text, err := svc.Finish(context.Background(), id, "u1")
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if text != "你好世界" {
		t.Errorf("expected transcription, got %q", text)
	}
}

func TestService_FinishEmptyAudio(t *testing.T) {
	svc, _ := newTestService(t, "x")

	id := svc.Start(context.Background(), "u1", "zh")
	if _, err := svc.Finish(context.Background(), id, "u1"); err != ErrEmptyAudio {
		t.Fatalf("expected ErrEmptyAudio, got %v", err)
	}
}

func TestService_FinishNotFound(t *testing.T) {
	svc, _ := newTestService(t, "x")
	if _, err := svc.Finish(context.Background(), "missing", "u1"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_FinishUnavailable(t *testing.T) {
	// Inert client (empty endpoint) + a session with audio → ErrUnavailable.
	svc := NewService(NewClient(ClientConfig{}), NewManager(ManagerConfig{}))
	id := svc.Start(context.Background(), "u1", "zh")
	if _, err := svc.Chunk(context.Background(), id, "u1", 0, []byte("pcm")); err != nil {
		t.Fatalf("chunk: %v", err)
	}
	if _, err := svc.Finish(context.Background(), id, "u1"); err != ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}
