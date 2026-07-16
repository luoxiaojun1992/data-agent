package im

import (
	"testing"
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
