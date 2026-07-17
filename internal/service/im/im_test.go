package im

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"hash"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
)

func TestNewService(t *testing.T) {
	cfg := Config{
		AppID:       "test-app-id",
		AppSecret:   "test-app-secret",
		VerifyToken: "test-verify-token",
	}
	s := NewService(cfg)
	if s == nil {
		t.Fatal("NewService() should not return nil")
	}
	if s.config.AppID != "test-app-id" {
		t.Errorf("AppID = %q, want %q", s.config.AppID, "test-app-id")
	}
}

func TestNewService_EmptyConfig(t *testing.T) {
	s := NewService(Config{})
	if s == nil {
		t.Fatal("NewService() should not return nil even with empty config")
	}
}

func TestConfig_Defaults(t *testing.T) {
	cfg := Config{
		AppID:       "id",
		AppSecret:   "secret",
		VerifyToken: "token",
	}
	if cfg.AppID != "id" {
		t.Errorf("AppID = %q, want %q", cfg.AppID, "id")
	}
}

func TestMessage_Defaults(t *testing.T) {
	msg := Message{
		OpenID:    "open123",
		UserID:    "user123",
		Text:      "Hello",
		Timestamp: "2024-01-01T00:00:00Z",
	}
	if msg.Text != "Hello" {
		t.Errorf("Text = %q, want %q", msg.Text, "Hello")
	}
}

func TestCardMessage_Defaults(t *testing.T) {
	card := CardMessage{
		Title:   "Test Card",
		Content: "Card content",
	}
	if card.Title != "Test Card" {
		t.Errorf("Title = %q, want %q", card.Title, "Test Card")
	}
}

func TestCardAction_Defaults(t *testing.T) {
	action := CardAction{
		Text: "Click me",
		URL:  "https://example.com",
		Type: "primary",
	}
	if action.Text != "Click me" {
		t.Errorf("Text = %q, want %q", action.Text, "Click me")
	}
	if action.Type != "primary" {
		t.Errorf("Type = %q, want %q", action.Type, "primary")
	}
}

func TestFormatCard(t *testing.T) {
	cfg := Config{
		AppID:       "test-app-id",
		AppSecret:   "test-secret",
		VerifyToken: "test-token",
	}
	s := NewService(cfg)

	card := CardMessage{
		Title:   "Test",
		Content: "Hello world",
		Actions: []CardAction{
			{Text: "Go", URL: "https://example.com", Type: "primary"},
		},
	}

	result := s.FormatCard(card)
	if result == nil {
		t.Fatal("FormatCard() should not return nil")
	}
	if msgType, ok := result["msg_type"]; !ok || msgType != "interactive" {
		t.Errorf("msg_type = %v, want 'interactive'", msgType)
	}
	cardField, ok := result["card"].(map[string]interface{})
	if !ok {
		t.Fatal("card should be a map")
	}
	header, ok := cardField["header"].(map[string]interface{})
	if !ok {
		t.Fatal("header should be a map")
	}
	title, ok := header["title"].(map[string]string)
	if !ok {
		t.Fatal("title should be a map")
	}
	if title["content"] != "Test" {
		t.Errorf("title content = %q, want %q", title["content"], "Test")
	}
}

func TestFormatCard_NoActions(t *testing.T) {
	cfg := Config{
		AppID:       "test-app-id",
		AppSecret:   "test-secret",
		VerifyToken: "test-token",
	}
	s := NewService(cfg)

	card := CardMessage{
		Title:   "Simple",
		Content: "Just content",
	}

	result := s.FormatCard(card)
	if result == nil {
		t.Fatal("FormatCard() should not return nil")
	}
}

func TestFormatCard_MultipleActions(t *testing.T) {
	cfg := Config{
		AppID:       "test-app-id",
		AppSecret:   "test-secret",
		VerifyToken: "test-token",
	}
	s := NewService(cfg)

	card := CardMessage{
		Title:   "Multi",
		Content: "Multiple actions",
		Actions: []CardAction{
			{Text: "Approve", URL: "https://example.com/approve", Type: "primary"},
			{Text: "Reject", URL: "https://example.com/reject", Type: "danger"},
		},
	}

	result := s.FormatCard(card)
	if result == nil {
		t.Fatal("FormatCard() should not return nil")
	}
}

func TestFormatTextMessage(t *testing.T) {
	cfg := Config{
		AppID:       "test-app-id",
		AppSecret:   "test-secret",
		VerifyToken: "test-token",
	}
	s := NewService(cfg)

	result := s.FormatTextMessage("Hello world")
	if result == nil {
		t.Fatal("FormatTextMessage() should not return nil")
	}
	if msgType, ok := result["msg_type"]; !ok || msgType != "text" {
		t.Errorf("msg_type = %v, want 'text'", msgType)
	}
	content, ok := result["content"].(map[string]interface{})
	if !ok {
		t.Fatal("content should be a map")
	}
	if content["text"] != "Hello world" {
		t.Errorf("text = %v, want 'Hello world'", content["text"])
	}
}

func TestFormatTextMessage_Empty(t *testing.T) {
	cfg := Config{
		AppID:       "test-app-id",
		AppSecret:   "test-secret",
		VerifyToken: "test-token",
	}
	s := NewService(cfg)

	result := s.FormatTextMessage("")
	if result == nil {
		t.Fatal("FormatTextMessage() should not return nil")
	}
}

func TestVerifySignature(t *testing.T) {
	cfg := Config{
		AppID:       "test-app-id",
		AppSecret:   "test-secret",
		VerifyToken: "test-token",
	}
	s := NewService(cfg)

	// Test with known values - should just not panic and return a boolean
	result := s.VerifySignature("1234567890", "nonce123", "some-signature")
	if result {
		// Could be true by coincidence, but that's fine
	}
	_ = result

	// Empty values should not panic
	s.VerifySignature("", "", "")
}

func TestStringsBuilder(t *testing.T) {
	var b stringsBuilder
	b.WriteString("hello")
	b.WriteString(" ")
	b.WriteString("world")
	if b.String() != "hello world" {
		t.Errorf("stringsBuilder.String() = %q, want %q", b.String(), "hello world")
	}
}

func TestStringsBuilder_Empty(t *testing.T) {
	var b stringsBuilder
	if b.String() != "" {
		t.Errorf("empty stringsBuilder.String() = %q, want %q", b.String(), "")
	}
}

// ========== gomonkey-based tests below ==========

// testHash implements hash.Hash for controlled output in tests.
type testHash struct {
	written [][]byte
	sumVal  []byte
}

func (h *testHash) Write(p []byte) (n int, err error) {
	h.written = append(h.written, p)
	return len(p), nil
}

func (h *testHash) Sum(b []byte) []byte {
	return append(b, h.sumVal...)
}

func (h *testHash) Reset() {}

func (h *testHash) Size() int { return len(h.sumVal) }

func (h *testHash) BlockSize() int { return 64 }

// TestVerifySignature_ValidSignature computes the expected HMAC-SHA256
// signature and verifies the function returns true for a match.
func TestVerifySignature_ValidSignature(t *testing.T) {
	secret := "my-secret"
	cfg := Config{AppSecret: secret}
	s := NewService(cfg)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("ts123" + "nonce456" + secret))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	if !s.VerifySignature("ts123", "nonce456", expected) {
		t.Error("valid signature should return true")
	}
}

// TestVerifySignature_InvalidSignature verifies the function returns false
// when provided with a wrong signature.
func TestVerifySignature_InvalidSignature(t *testing.T) {
	cfg := Config{AppSecret: "my-secret"}
	s := NewService(cfg)

	if s.VerifySignature("ts123", "nonce456", "wrong-signature") {
		t.Error("wrong signature should return false")
	}
}

// TestVerifySignature_DifferentSecret verifies that a signature generated
// with a different secret does not match.
func TestVerifySignature_DifferentSecret(t *testing.T) {
	cfg := Config{AppSecret: "secret-a"}
	s := NewService(cfg)

	// Generate signature with secret-b
	mac := hmac.New(sha256.New, []byte("secret-b"))
	mac.Write([]byte("ts" + "nc" + "secret-b"))
	wrongSign := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	if s.VerifySignature("ts", "nc", wrongSign) {
		t.Error("signature from different secret should not match")
	}
}

// TestVerifySignature_EmptyInputs verifies signature verification works
// with empty timestamp and nonce.
func TestVerifySignature_EmptyInputs(t *testing.T) {
	secret := "my-secret"
	cfg := Config{AppSecret: secret}
	s := NewService(cfg)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("" + "" + secret))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	if !s.VerifySignature("", "", expected) {
		t.Error("signature for empty inputs should be valid")
	}
}

// TestVerifySignature_WithGomonkey uses gomonkey to mock hmac.New and
// verify that the correct secret key is propagated.
func TestVerifySignature_WithGomonkey(t *testing.T) {
	secret := "my-app-secret"
	cfg := Config{AppSecret: secret}
	s := NewService(cfg)

	var capturedKey []byte
	fixedSum := []byte("gomonkey-fixed-hash-output")
	patches := gomonkey.ApplyFunc(hmac.New, func(h func() hash.Hash, key []byte) hash.Hash {
		capturedKey = make([]byte, len(key))
		copy(capturedKey, key)
		return &testHash{sumVal: fixedSum}
	})
	defer patches.Reset()

	s.VerifySignature("ts", "nc", "any-signature")

	if string(capturedKey) != secret {
		t.Errorf("hmac.New key = %q, want %q", string(capturedKey), secret)
	}
}

// TestVerifySignature_MockedHash uses gomonkey to mock hmac.New to return
// a testHash with a known Sum, then verifies signature matching logic.
func TestVerifySignature_MockedHash(t *testing.T) {
	cfg := Config{AppSecret: "any-secret"}
	s := NewService(cfg)

	fixedHashOutput := []byte("fixed-hash-output")

	patches := gomonkey.ApplyFunc(hmac.New, func(h func() hash.Hash, key []byte) hash.Hash {
		return &testHash{sumVal: fixedHashOutput}
	})
	defer patches.Reset()

	expectedSign := base64.StdEncoding.EncodeToString(fixedHashOutput)

	if !s.VerifySignature("ts", "nc", expectedSign) {
		t.Error("signature should match when using mocked hash output")
	}

	if s.VerifySignature("ts", "nc", "different-signature") {
		t.Error("different signature should not match")
	}
}

// mockResponseWriter captures HTTP responses for testing.
type mockResponseWriter struct {
	header        http.Header
	body          []byte
	statusCode    int
	writeErr      error
	writeErrCalled bool
}

func (w *mockResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *mockResponseWriter) Write(b []byte) (int, error) {
	if w.writeErr != nil {
		w.writeErrCalled = true
		return 0, w.writeErr
	}
	w.body = append(w.body, b...)
	return len(b), nil
}

func (w *mockResponseWriter) WriteHeader(statusCode int) {
	if w.statusCode == 0 {
		w.statusCode = statusCode
	}
}

// TestWebhookHandler_URLVerification tests the Feishu URL verification
// challenge using gomonkey to mock io.ReadAll.
func TestWebhookHandler_URLVerification(t *testing.T) {
	challengeBody := `{"type":"url_verification","challenge":"challenge_token_abc","token":"verify_token_xyz"}`

	patches := gomonkey.ApplyFunc(io.ReadAll, func(r io.Reader) ([]byte, error) {
		return []byte(challengeBody), nil
	})
	defer patches.Reset()

	cfg := Config{AppID: "app-id", AppSecret: "app-secret", VerifyToken: "verify-token"}
	s := NewService(cfg)

	w := &mockResponseWriter{}
	r, _ := http.NewRequest(http.MethodPost, "/webhook", nil)

	handler := s.WebhookHandler()
	handler(w, r)

	if w.statusCode != 0 && w.statusCode != 200 {
		t.Errorf("unexpected status code %d", w.statusCode)
	}
	if !contains(string(w.body), "challenge_token_abc") {
		t.Errorf("response body = %q, expected to contain challenge token", string(w.body))
	}
}

// TestWebhookHandler_MessageEvent tests handling of a message event
// using gomonkey to mock io.ReadAll and time.Now.
func TestWebhookHandler_MessageEvent(t *testing.T) {
	eventBody := `{
		"header": {"event_type": "im.message.receive_v1"},
		"event": {
			"sender": {"sender_id": {"open_id": "user_open_123"}},
			"message": {"content": "Hello Feishu"}
		}
	}`

	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	patches := gomonkey.ApplyFunc(io.ReadAll, func(r io.Reader) ([]byte, error) {
		return []byte(eventBody), nil
	})
	defer patches.Reset()

	patches.ApplyFunc(time.Now, func() time.Time {
		return fixedTime
	})

	cfg := Config{AppID: "app-id", AppSecret: "app-secret", VerifyToken: "verify-token"}
	s := NewService(cfg)

	w := &mockResponseWriter{}
	r, _ := http.NewRequest(http.MethodPost, "/webhook", nil)

	handler := s.WebhookHandler()
	handler(w, r)

	if !contains(string(w.body), "收到消息: Hello Feishu") {
		t.Errorf("response should contain echoed message, got: %q", string(w.body))
	}
	if !contains(string(w.body), "msg_type") {
		t.Errorf("response should contain msg_type field")
	}
}

// TestWebhookHandler_MethodNotAllowed verifies that non-POST requests
// return 405 Method Not Allowed.
func TestWebhookHandler_MethodNotAllowed(t *testing.T) {
	cfg := Config{AppID: "app-id", AppSecret: "secret", VerifyToken: "token"}
	s := NewService(cfg)

	w := &mockResponseWriter{}
	r, _ := http.NewRequest(http.MethodGet, "/webhook", nil)

	handler := s.WebhookHandler()
	handler(w, r)

	if w.statusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.statusCode, http.StatusMethodNotAllowed)
	}
}

// TestWebhookHandler_ReadBodyError verifies that a read error returns
// 400 Bad Request using gomonkey to mock io.ReadAll.
func TestWebhookHandler_ReadBodyError(t *testing.T) {
	patches := gomonkey.ApplyFunc(io.ReadAll, func(r io.Reader) ([]byte, error) {
		return nil, io.ErrUnexpectedEOF
	})
	defer patches.Reset()

	cfg := Config{AppID: "app-id", AppSecret: "secret", VerifyToken: "token"}
	s := NewService(cfg)

	w := &mockResponseWriter{}
	r, _ := http.NewRequest(http.MethodPost, "/webhook", nil)

	handler := s.WebhookHandler()
	handler(w, r)

	if w.statusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.statusCode, http.StatusBadRequest)
	}
}

// TestWebhookHandler_InvalidJSON tests handling of invalid JSON in the
// request body using gomonkey to mock io.ReadAll.
func TestWebhookHandler_InvalidJSON(t *testing.T) {
	patches := gomonkey.ApplyFunc(io.ReadAll, func(r io.Reader) ([]byte, error) {
		return []byte(`not valid json`), nil
	})
	defer patches.Reset()

	cfg := Config{AppID: "app-id", AppSecret: "secret", VerifyToken: "token"}
	s := NewService(cfg)

	w := &mockResponseWriter{}
	r, _ := http.NewRequest(http.MethodPost, "/webhook", nil)

	handler := s.WebhookHandler()
	handler(w, r)

	if w.statusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.statusCode, http.StatusBadRequest)
	}
}

// TestWebhookHandler_EmptyBody tests handling of an empty request body.
func TestWebhookHandler_EmptyBody(t *testing.T) {
	patches := gomonkey.ApplyFunc(io.ReadAll, func(r io.Reader) ([]byte, error) {
		return []byte(`{}`), nil
	})
	defer patches.Reset()

	cfg := Config{AppID: "app-id", AppSecret: "secret", VerifyToken: "token"}
	s := NewService(cfg)

	w := &mockResponseWriter{}
	r, _ := http.NewRequest(http.MethodPost, "/webhook", nil)

	handler := s.WebhookHandler()
	handler(w, r)

	// Empty body should be handled gracefully - it will try to parse
	// as challenge (won't match), then as event (won't match header),
	// and will echo back with empty message text
	if !contains(string(w.body), "msg_type") {
		t.Errorf("response should contain msg_type, got: %q", string(w.body))
	}
}

// TestWebhookHandler_EncodeError_URLVerification covers the json.Encode error path
// in the URL verification response (line 132-134).
func TestWebhookHandler_EncodeError_URLVerification(t *testing.T) {
	challengeBody := `{"type":"url_verification","challenge":"challenge_token_abc","token":"verify_token_xyz"}`

	patches := gomonkey.ApplyFunc(io.ReadAll, func(r io.Reader) ([]byte, error) {
		return []byte(challengeBody), nil
	})
	defer patches.Reset()

	cfg := Config{AppID: "app-id", AppSecret: "app-secret", VerifyToken: "verify-token"}
	s := NewService(cfg)

	// Mock Writer.Write to return an error (simulating encode failure)
	w := &mockResponseWriter{
		writeErr: io.ErrClosedPipe,
	}
	r, _ := http.NewRequest(http.MethodPost, "/webhook", nil)

	handler := s.WebhookHandler()
	handler(w, r)

	// Should not panic; error is logged but not propagated
	if w.writeErrCalled {
		// Success - write error was triggered
	}
}

// TestWebhookHandler_EncodeError_MessageEvent covers the json.Encode error path
// in the message event response (line 169-170).
func TestWebhookHandler_EncodeError_MessageEvent(t *testing.T) {
	eventBody := `{
		"header": {"event_type": "im.message.receive_v1"},
		"event": {
			"sender": {"sender_id": {"open_id": "user_open_123"}},
			"message": {"content": "Hello"}
		}
	}`

	patches := gomonkey.ApplyFunc(io.ReadAll, func(r io.Reader) ([]byte, error) {
		return []byte(eventBody), nil
	})
	defer patches.Reset()

	cfg := Config{AppID: "app-id", AppSecret: "app-secret", VerifyToken: "verify-token"}
	s := NewService(cfg)

	w := &mockResponseWriter{
		writeErr: io.ErrClosedPipe,
	}
	r, _ := http.NewRequest(http.MethodPost, "/webhook", nil)

	handler := s.WebhookHandler()
	handler(w, r)

	// Should not panic; error is logged but not propagated
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
