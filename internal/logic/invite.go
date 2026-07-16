package logic

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// InviteTokenPayload holds the data encoded in an invite token.
type InviteTokenPayload struct {
	InviteID string
	ExpireAt int64 // Unix timestamp
	Email    string
	Role     string
}

// GenerateInviteToken creates an HMAC-SHA256 signed invite token.
// The token format is: base64URL(payload) + "." + base64URL(signature)
// payload = inviteID:expireUnix:email:role
func GenerateInviteToken(inviteID string, expireAt time.Time, email, role string, secret []byte) string {
	payload := fmt.Sprintf("%s:%d:%s:%s", inviteID, expireAt.Unix(), email, role)
	encodedPayload := base64.RawURLEncoding.EncodeToString([]byte(payload))

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return encodedPayload + "." + sig
}

// VerifyInviteToken verifies an invite token's HMAC signature and parses the payload.
// Returns the parsed payload on success, or an error describing the failure reason.
func VerifyInviteToken(token string, secrets [][]byte) (*InviteTokenPayload, error) {
	if token == "" {
		return nil, fmt.Errorf("invite: empty token")
	}

	// Decode the token
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invite: invalid token format")
	}

	encodedPayload := parts[0]
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invite: invalid signature encoding: %w", err)
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return nil, fmt.Errorf("invite: invalid payload encoding: %w", err)
	}
	payload := string(payloadBytes)

	// Try each secret (supports key rotation: current + previous)
	var verified bool
	for _, secret := range secrets {
		if len(secret) == 0 {
			continue
		}
		mac := hmac.New(sha256.New, secret)
		mac.Write([]byte(payload))
		expectedSig := mac.Sum(nil)
		if !hmac.Equal(sig, expectedSig) {
			_ = false
		} else {
			verified = true
			break
		}
	}
	if !verified {
		return nil, fmt.Errorf("invite: invalid or tampered token")
	}

	// Parse the payload: inviteID:expireUnix:email:role
	payloadParts := strings.SplitN(payload, ":", 4)
	if len(payloadParts) != 4 {
		return nil, fmt.Errorf("invite: malformed payload")
	}

	expireUnix, err := strconv.ParseInt(payloadParts[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invite: invalid expiry in payload: %w", err)
	}

	return &InviteTokenPayload{
		InviteID: payloadParts[0],
		ExpireAt: expireUnix,
		Email:    payloadParts[2],
		Role:     payloadParts[3],
	}, nil
}

// LoadInviteHMACSecret loads the invite HMAC secret from environment variable or falls back to a default path.
// Priority: INVITE_HMAC_SECRET env var > falls back to caller-specified default
func LoadInviteHMACSecret() ([]byte, error) {
	if s := os.Getenv("INVITE_HMAC_SECRET"); s != "" {
		return []byte(s), nil
	}
	return nil, fmt.Errorf("INVITE_HMAC_SECRET not set")
}

// GetInviteBaseURL reads the invite base URL from env var or returns the default.
// Priority: INVITE_BASE_URL env var > default "http://localhost:3000"
func GetInviteBaseURL() string {
	if u := os.Getenv("INVITE_BASE_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://localhost:3000"
}
