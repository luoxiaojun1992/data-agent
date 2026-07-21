package im

//go:generate mockery --name IMService --output ./mocks --outpkg mocks

import (
	"net/http"
)

// IMService defines the IM integration service contract.
type IMService interface {
	VerifySignature(timestamp, nonce, sign string) bool
	FormatCard(card CardMessage) map[string]interface{}
	FormatTextMessage(text string) map[string]interface{}
	WebhookHandler() http.HandlerFunc
}
