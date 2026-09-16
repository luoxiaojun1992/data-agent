package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	rbacsvc "github.com/luoxiaojun1992/data-agent/internal/service/rbac"
	"github.com/luoxiaojun1992/data-agent/internal/service/voice"
)

// voiceIntegrationRouter wires the real JWT auth + RBAC permission middleware +
// voice routes onto a gin engine, fronting a fake sidecar that echoes the
// received PCM byte length back as the transcription. It exercises the full
// main-backend API path: HTTP → JWT → RBAC → handler → service → client → sidecar.
func voiceIntegrationRouter(t *testing.T, allowPerm bool) (*gin.Engine, *httptest.Server, *int) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// Fake sidecar: record the decoded PCM length, return it as the "text" so the
	// test can assert the audio reached the sidecar byte-for-byte.
	var receivedLen int
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Audio string `json:"audio"`
			Lang  string `json:"lang"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		raw, err := base64.StdEncoding.DecodeString(req.Audio)
		if err != nil {
			http.Error(w, "bad base64", http.StatusBadRequest)
			return
		}
		receivedLen = len(raw)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "集成转写成功"})
	}))
	t.Cleanup(sidecar.Close)

	client := voice.NewClient(voice.ClientConfig{Endpoint: sidecar.URL})
	manager := voice.NewManager(voice.ManagerConfig{})
	svc := voice.NewService(client, manager)
	h := NewVoiceHandler(svc)

	jwtMgr := middleware.NewJWTManager("test-secret", time.Hour)

	rbacSvc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(rbacSvc, "HasPermission",
		func(_ context.Context, _ string, _ string) (bool, error) { return allowPerm, nil })
	t.Cleanup(patches.Reset)

	r := gin.New()
	group := r.Group("/api/v1/voice")
	group.Use(jwtMgr.AuthMiddleware(), middleware.RequirePermission(rbacSvc, model.PermChatView))
	RegisterVoiceRoutes(group, h)
	return r, sidecar, &receivedLen
}

func voiceToken(t *testing.T) string {
	t.Helper()
	m := middleware.NewJWTManager("test-secret", time.Hour)
	tok, err := m.GenerateToken("u1", "alice", "user")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return tok
}

func voicePost(r *gin.Engine, path, body, token string) (*httptest.ResponseRecorder, []byte) {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w, w.Body.Bytes()
}

// TestVoiceIntegration_FullPath drives start → chunk → finish through the real
// middleware stack and asserts the transcription is returned verbatim.
func TestVoiceIntegration_FullPath(t *testing.T) {
	r, _, receivedLen := voiceIntegrationRouter(t, true)
	tok := voiceToken(t)

	// start
	w, body := voicePost(r, "/api/v1/voice/start", `{"lang":"zh"}`, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("start: %d %s", w.Code, body)
	}
	var start struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(body, &start); err != nil || start.RequestID == "" {
		t.Fatalf("start: bad body %s", body)
	}

	// chunk (seq 0, 8 bytes of PCM)
	audio := base64.StdEncoding.EncodeToString([]byte("12345678"))
	w, body = voicePost(r, "/api/v1/voice/chunk",
		`{"request_id":"`+start.RequestID+`","seq":0,"audio":"`+audio+`"}`, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("chunk: %d %s", w.Code, body)
	}

	// finish
	w, body = voicePost(r, "/api/v1/voice/finish",
		`{"request_id":"`+start.RequestID+`"}`, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("finish: %d %s", w.Code, body)
	}
	var finish struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &finish); err != nil {
		t.Fatalf("finish: bad body %s", body)
	}
	if finish.Text != "集成转写成功" {
		t.Errorf("finish text = %q, want 集成转写成功", finish.Text)
	}
	if *receivedLen != 8 {
		t.Errorf("sidecar received %d PCM bytes, want 8", *receivedLen)
	}
}

// TestVoiceIntegration_NoToken asserts the auth middleware rejects requests
// without a Bearer token.
func TestVoiceIntegration_NoToken(t *testing.T) {
	r, _, _ := voiceIntegrationRouter(t, true)
	w, _ := voicePost(r, "/api/v1/voice/start", `{"lang":"zh"}`, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestVoiceIntegration_NoPermission asserts the RBAC middleware rejects a user
// lacking chat:view.
func TestVoiceIntegration_NoPermission(t *testing.T) {
	r, _, _ := voiceIntegrationRouter(t, false)
	tok := voiceToken(t)
	w, body := voicePost(r, "/api/v1/voice/start", `{"lang":"zh"}`, tok)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, body)
	}
}

// TestVoiceIntegration_OwnershipIsolation asserts user B cannot chunk/finish
// user A's session (IDOR guard).
func TestVoiceIntegration_OwnershipIsolation(t *testing.T) {
	r, _, _ := voiceIntegrationRouter(t, true)
	tokA := voiceToken(t) // user u1

	m := middleware.NewJWTManager("test-secret", time.Hour)
	tokB, _ := m.GenerateToken("u2", "bob", "user")

	w, body := voicePost(r, "/api/v1/voice/start", `{"lang":"zh"}`, tokA)
	var start struct {
		RequestID string `json:"request_id"`
	}
	_ = json.Unmarshal(body, &start)

	audio := base64.StdEncoding.EncodeToString([]byte("1234"))
	w, body = voicePost(r, "/api/v1/voice/chunk",
		`{"request_id":"`+start.RequestID+`","seq":0,"audio":"`+audio+`"}`, tokB)
	if w.Code != http.StatusNotFound {
		t.Fatalf("user B chunk: expected 404, got %d: %s", w.Code, body)
	}

	w, body = voicePost(r, "/api/v1/voice/finish",
		`{"request_id":"`+start.RequestID+`"}`, tokB)
	if w.Code != http.StatusNotFound {
		t.Fatalf("user B finish: expected 404, got %d: %s", w.Code, body)
	}
}

// TestVoiceIntegration_SeqOrder asserts a chunk with a non-contiguous seq is
// rejected with 409 through the full stack.
func TestVoiceIntegration_SeqOrder(t *testing.T) {
	r, _, _ := voiceIntegrationRouter(t, true)
	tok := voiceToken(t)

	w, body := voicePost(r, "/api/v1/voice/start", `{"lang":"zh"}`, tok)
	var start struct {
		RequestID string `json:"request_id"`
	}
	_ = json.Unmarshal(body, &start)

	audio := base64.StdEncoding.EncodeToString([]byte("1234"))
	w, body = voicePost(r, "/api/v1/voice/chunk",
		`{"request_id":"`+start.RequestID+`","seq":3,"audio":"`+audio+`"}`, tok)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, body)
	}
}
