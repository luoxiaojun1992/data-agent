package logic

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateInviteToken(t *testing.T) {
	secret := []byte("test-secret-key-32")
	inviteID := "inv_test123"
	email := "test@example.com"
	role := "user"
	expireAt := time.Now().Add(24 * time.Hour)

	token := GenerateInviteToken(inviteID, expireAt, email, role, secret)

	t.Run("generates non-empty token", func(t *testing.T) {
		if token == "" {
			t.Error("token should not be empty")
		}
	})

	t.Run("token has correct format (payload.sig)", func(t *testing.T) {
		parts := strings.Split(token, ".")
		if len(parts) != 2 {
			t.Errorf("token should have 2 parts separated by '.', got %d parts", len(parts))
		}
	})

	t.Run("same inputs produce same token", func(t *testing.T) {
		token2 := GenerateInviteToken(inviteID, expireAt, email, role, secret)
		if token != token2 {
			t.Error("same inputs should produce identical tokens")
		}
	})

	t.Run("different secret produces different token", func(t *testing.T) {
		token2 := GenerateInviteToken(inviteID, expireAt, email, role, []byte("different-secret-key"))
		if token == token2 {
			t.Error("different secrets should produce different tokens")
		}
	})
}

func TestVerifyInviteToken(t *testing.T) {
	secret := []byte("test-secret-key-32")
	inviteID := "inv_test123"
	email := "test@example.com"
	role := "user"
	expireAt := time.Now().Add(24 * time.Hour)

	t.Run("valid token verifies", func(t *testing.T) {
		token := GenerateInviteToken(inviteID, expireAt, email, role, secret)
		payload, err := VerifyInviteToken(token, [][]byte{secret})
		if err != nil {
			t.Fatalf("VerifyInviteToken unexpected error: %v", err)
		}
		if payload.InviteID != inviteID {
			t.Errorf("payload.InviteID: got %s, want %s", payload.InviteID, inviteID)
		}
		if payload.Email != email {
			t.Errorf("payload.Email: got %s, want %s", payload.Email, email)
		}
		if payload.Role != role {
			t.Errorf("payload.Role: got %s, want %s", payload.Role, role)
		}
	})

	t.Run("wrong secret rejects", func(t *testing.T) {
		token := GenerateInviteToken(inviteID, expireAt, email, role, secret)
		_, err := VerifyInviteToken(token, [][]byte{[]byte("wrong-secret")})
		if err == nil {
			t.Error("wrong secret should reject token")
		}
	})

	t.Run("tampered token rejects", func(t *testing.T) {
		token := GenerateInviteToken(inviteID, expireAt, email, role, secret)
		tampered := token[:len(token)-2] + "AA"
		_, err := VerifyInviteToken(tampered, [][]byte{secret})
		if err == nil {
			t.Error("tampered token should be rejected")
		}
	})

	t.Run("empty token rejects", func(t *testing.T) {
		_, err := VerifyInviteToken("", [][]byte{secret})
		if err == nil {
			t.Error("empty token should be rejected")
		}
	})

	t.Run("invalid format rejects", func(t *testing.T) {
		_, err := VerifyInviteToken("not-a-valid-token", [][]byte{secret})
		if err == nil {
			t.Error("invalid format should be rejected")
		}
	})

	t.Run("key rotation: second secret works", func(t *testing.T) {
		oldSecret := []byte("old-secret-key-x")
		newSecret := []byte("new-secret-key-x")
		token := GenerateInviteToken(inviteID, expireAt, email, role, newSecret)
		payload, err := VerifyInviteToken(token, [][]byte{oldSecret, newSecret})
		if err != nil {
			t.Fatalf("key rotation: %v", err)
		}
		if payload.InviteID != inviteID {
			t.Errorf("key rotation payload: got %s", payload.InviteID)
		}
	})

	t.Run("malformed payload rejects", func(t *testing.T) {
		// Manually construct a token with bad payload
		token := GenerateInviteToken(inviteID, expireAt, email, role, secret)
		parts := strings.Split(token, ".")
		badToken := "YWJj" + "." + parts[1] // "abc" in base64
		_, err := VerifyInviteToken(badToken, [][]byte{secret})
		if err == nil {
			t.Error("malformed payload should reject")
		}
	})
}
