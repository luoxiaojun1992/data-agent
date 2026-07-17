package im

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"
)

// Config holds Feishu IM configuration.
type Config struct {
	AppID       string
	AppSecret   string
	VerifyToken string
}

// Message represents an incoming Feishu message.
type Message struct {
	OpenID    string `json:"open_id"`
	UserID    string `json:"user_id"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
}

// CardMessage represents a Feishu interactive card.
type CardMessage struct {
	Title   string       `json:"title"`
	Content string       `json:"content"`
	Actions []CardAction `json:"actions,omitempty"`
}

// CardAction represents an action button in a card.
type CardAction struct {
	Text string `json:"text"`
	URL  string `json:"url"`
	Type string `json:"type"` // "primary", "default", "danger"
}

// Service handles Feishu IM integration.
type Service struct {
	config Config
}

// NewService creates a new IM service.
func NewService(cfg Config) *Service {
	return &Service{config: cfg}
}

// VerifySignature validates the Feishu webhook signature.
func (s *Service) VerifySignature(timestamp, nonce, sign string) bool {
	var b stringsBuilder
	b.WriteString(timestamp)
	b.WriteString(nonce)
	b.WriteString(s.config.AppSecret)
	mac := hmac.New(sha256.New, []byte(s.config.AppSecret))
	mac.Write([]byte(b.String()))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return sign == expected
}

// FormatCard formats a response as a Feishu interactive card JSON.
func (s *Service) FormatCard(card CardMessage) map[string]interface{} {
	elements := []map[string]interface{}{
		{
			"tag":     "markdown",
			"content": card.Content,
		},
	}

	for _, action := range card.Actions {
		elements = append(elements, map[string]interface{}{
			"tag": "action",
			"actions": []map[string]interface{}{
				{
					"tag":  "button",
					"text": map[string]string{"tag": "plain_text", "content": action.Text},
					"url":  action.URL,
					"type": action.Type,
				},
			},
		})
	}

	return map[string]interface{}{
		"msg_type": "interactive",
		"card": map[string]interface{}{
			"header": map[string]interface{}{
				"title":    map[string]string{"tag": "plain_text", "content": card.Title},
				"template": "blue",
			},
			"elements": elements,
		},
	}
}

// FormatTextMessage formats a simple text response.
func (s *Service) FormatTextMessage(text string) map[string]interface{} {
	return map[string]interface{}{
		"msg_type": "text",
		"content": map[string]interface{}{
			"text": text,
		},
	}
}

// WebhookHandler returns an HTTP handler for Feishu webhook events.
func (s *Service) WebhookHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body failed", http.StatusBadRequest)
			return
		}

		// Verify Feishu challenge
		var challenge struct {
			Type      string `json:"type"`
			Challenge string `json:"challenge"`
			Token     string `json:"token"`
		}
		if err := json.Unmarshal(body, &challenge); err == nil && challenge.Type == "url_verification" {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]string{"challenge": challenge.Challenge}); err != nil {
				log.Printf("feishu webhook: encode challenge response error: %v", err)
			}
			return
		}

		// Parse event
		var event struct {
			Header struct {
				EventType string `json:"event_type"`
			} `json:"header"`
			Event struct {
				Sender struct {
					SenderID struct {
						OpenID string `json:"open_id"`
					} `json:"sender_id"`
				} `json:"sender"`
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"event"`
		}

		if err := json.Unmarshal(body, &event); err != nil {
			http.Error(w, "parse event failed", http.StatusBadRequest)
			return
		}

		// Echo back for MVP
		msg := Message{
			OpenID:    event.Event.Sender.SenderID.OpenID,
			Text:      event.Event.Message.Content,
			Timestamp: time.Now().Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		resp := s.FormatTextMessage("收到消息: " + msg.Text)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("feishu webhook: encode response error: %v", err)
		}
	}
}

type stringsBuilder struct {
	data string
}

func (b *stringsBuilder) WriteString(s string) {
	b.data += s
}

func (b *stringsBuilder) String() string {
	return b.data
}
