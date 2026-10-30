package handler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luoxiaojun1992/data-agent/internal/service/voice"
)

// ── Chunk input validation ──

func TestVoiceHandler_ChunkBadJSON(t *testing.T) {
	h := newVoiceHandler(t)
	c, w := voiceGin(`not-json`)
	h.Chunk(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestVoiceHandler_ChunkEmptyRequestID(t *testing.T) {
	h := newVoiceHandler(t)
	c, w := voiceGin(`{"request_id":"","seq":0,"audio":"cGNt"}`)
	h.Chunk(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestVoiceHandler_ChunkEmptyAudio(t *testing.T) {
	h := newVoiceHandler(t)
	c, w := voiceGin(`{"request_id":"r","seq":0,"audio":""}`)
	h.Chunk(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestVoiceHandler_ChunkBadBase64(t *testing.T) {
	h := newVoiceHandler(t)
	c, w := voiceGin(`{"request_id":"r","seq":0,"audio":"!!!not-base64!!!"}`)
	h.Chunk(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── Chunk ErrTooLarge (per-chunk cap) ──

func TestVoiceHandler_ChunkTooLarge(t *testing.T) {
	manager := voice.NewManager(voice.ManagerConfig{MaxChunk: 4})
	svc := voice.NewService(voice.NewClient(voice.ClientConfig{}), manager)
	h := NewVoiceHandler(svc)

	c, w := voiceGin(`{"lang":"zh"}`)
	h.Start(c)
	var startResp struct {
		RequestID string `json:"request_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &startResp)

	audio := base64.StdEncoding.EncodeToString([]byte("12345678")) // 8 bytes > 4
	c, w = voiceGin(`{"request_id":"` + startResp.RequestID + `","seq":0,"audio":"` + audio + `"}`)
	h.Chunk(c)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
}

// ── Finish input validation ──

func TestVoiceHandler_FinishBadJSON(t *testing.T) {
	h := newVoiceHandler(t)
	c, w := voiceGin(`not-json`)
	h.Finish(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestVoiceHandler_FinishEmptyRequestID(t *testing.T) {
	h := newVoiceHandler(t)
	c, w := voiceGin(`{"request_id":""}`)
	h.Finish(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── Finish error mapping (sidecar unavailable / sidecar error field) ──

func voiceHandlerWithSidecar(t *testing.T, handler http.HandlerFunc) *VoiceHandler {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client := voice.NewClient(voice.ClientConfig{Endpoint: srv.URL})
	return NewVoiceHandler(voice.NewService(client, voice.NewManager(voice.ManagerConfig{})))
}

func startAndChunk(t *testing.T, h *VoiceHandler) string {
	t.Helper()
	c, w := voiceGin(`{"lang":"zh"}`)
	h.Start(c)
	var startResp struct {
		RequestID string `json:"request_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &startResp)

	c, w = voiceGin(`{"request_id":"` + startResp.RequestID + `","seq":0,"audio":"cGNt"}`)
	h.Chunk(c)
	if w.Code != http.StatusOK {
		t.Fatalf("chunk setup failed: %d: %s", w.Code, w.Body.String())
	}
	return startResp.RequestID
}

func TestVoiceHandler_FinishUnavailable(t *testing.T) {
	h := voiceHandlerWithSidecar(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	requestID := startAndChunk(t, h)

	c, w := voiceGin(`{"request_id":"` + requestID + `"}`)
	h.Finish(c)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestVoiceHandler_FinishSidecarError(t *testing.T) {
	h := voiceHandlerWithSidecar(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	})
	requestID := startAndChunk(t, h)

	c, w := voiceGin(`{"request_id":"` + requestID + `"}`)
	h.Finish(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}
