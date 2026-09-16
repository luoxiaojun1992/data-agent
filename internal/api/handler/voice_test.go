package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/service/voice"
)

// newVoiceHandler wires a VoiceHandler backed by a real session manager and a
// fake sidecar that echoes a fixed transcription.
func newVoiceHandler(t *testing.T) *VoiceHandler {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"转写结果"}`))
	}))
	t.Cleanup(srv.Close)

	client := voice.NewClient(voice.ClientConfig{Endpoint: srv.URL})
	manager := voice.NewManager(voice.ManagerConfig{})
	return NewVoiceHandler(voice.NewService(client, manager))
}

func voiceGin(body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/voice/start", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", "u1")
	return c, w
}

func TestVoiceHandler_StartChunkFinish(t *testing.T) {
	h := newVoiceHandler(t)

	// start
	c, w := voiceGin(`{"lang":"zh"}`)
	h.Start(c)
	if w.Code != http.StatusOK {
		t.Fatalf("start: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var startResp struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &startResp); err != nil || startResp.RequestID == "" {
		t.Fatalf("start: bad response %s", w.Body.String())
	}

	// chunk (seq 0)
	c, w = voiceGin(`{"request_id":"` + startResp.RequestID + `","seq":0,"audio":"cGNt"}`)
	h.Chunk(c)
	if w.Code != http.StatusOK {
		t.Fatalf("chunk: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// finish
	c, w = voiceGin(`{"request_id":"` + startResp.RequestID + `"}`)
	h.Finish(c)
	if w.Code != http.StatusOK {
		t.Fatalf("finish: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "转写结果") {
		t.Errorf("finish: expected transcription, got %s", w.Body.String())
	}
}

func TestVoiceHandler_ChunkSeqConflict(t *testing.T) {
	h := newVoiceHandler(t)

	c, w := voiceGin(`{"lang":"zh"}`)
	h.Start(c)
	var startResp struct {
		RequestID string `json:"request_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &startResp)

	// seq 5 (skipping 0) → 409
	c, w = voiceGin(`{"request_id":"` + startResp.RequestID + `","seq":5,"audio":"cGNt"}`)
	h.Chunk(c)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestVoiceHandler_ChunkNotFound(t *testing.T) {
	h := newVoiceHandler(t)

	c, w := voiceGin(`{"request_id":"missing","seq":0,"audio":"cGNt"}`)
	h.Chunk(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestVoiceHandler_FinishNotFound(t *testing.T) {
	h := newVoiceHandler(t)

	c, w := voiceGin(`{"request_id":"missing"}`)
	h.Finish(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestVoiceHandler_FinishEmptyAudio(t *testing.T) {
	h := newVoiceHandler(t)

	c, w := voiceGin(`{"lang":"zh"}`)
	h.Start(c)
	var startResp struct {
		RequestID string `json:"request_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &startResp)

	// finish without any chunk → 400
	c, w = voiceGin(`{"request_id":"` + startResp.RequestID + `"}`)
	h.Finish(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestVoiceHandler_StartInvalidBody(t *testing.T) {
	h := newVoiceHandler(t)

	c, w := voiceGin(`not-json`)
	h.Start(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRegisterVoiceRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := newVoiceHandler(t)
	rg := r.Group("/api/v1/voice")
	RegisterVoiceRoutes(rg, h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/voice/start", strings.NewReader(`{"lang":"zh"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
