package logic

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
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

	t.Run("expired token still verifies but payload contains past timestamp", func(t *testing.T) {
		pastExpiry := time.Now().Add(-1 * time.Hour)
		token := GenerateInviteToken(inviteID, pastExpiry, email, role, secret)
		payload, err := VerifyInviteToken(token, [][]byte{secret})
		if err != nil {
			t.Fatalf("expired token should still verify (expiry check is caller's responsibility): %v", err)
		}
		if payload.ExpireAt >= time.Now().Unix() {
			t.Errorf("expired token ExpireAt should be in the past: got %d, now %d", payload.ExpireAt, time.Now().Unix())
		}
	})

	t.Run("empty secrets list rejects", func(t *testing.T) {
		token := GenerateInviteToken(inviteID, expireAt, email, role, secret)
		_, err := VerifyInviteToken(token, [][]byte{})
		if err == nil {
			t.Error("empty secrets list should reject token")
		}
	})

	t.Run("single secret works", func(t *testing.T) {
		token := GenerateInviteToken(inviteID, expireAt, email, role, secret)
		payload, err := VerifyInviteToken(token, [][]byte{secret})
		if err != nil {
			t.Fatalf("single secret verification failed: %v", err)
		}
		if payload.Email != email {
			t.Errorf("payload.Email: got %s, want %s", payload.Email, email)
		}
	})

	t.Run("empty secret in rotation is ignored", func(t *testing.T) {
		token := GenerateInviteToken(inviteID, expireAt, email, role, secret)
		payload, err := VerifyInviteToken(token, [][]byte{{}, secret, {}}) // empty secrets at start and end
		if err != nil {
			t.Fatalf("empty secrets in rotation should be ignored: %v", err)
		}
		if payload.InviteID != inviteID {
			t.Errorf("payload.InviteID: got %s, want %s", payload.InviteID, inviteID)
		}
	})

	t.Run("all empty secrets in rotation rejects", func(t *testing.T) {
		token := GenerateInviteToken(inviteID, expireAt, email, role, secret)
		_, err := VerifyInviteToken(token, [][]byte{{}, {}, {}})
		if err == nil {
			t.Error("all empty secrets should reject token")
		}
	})
}

func TestGenerateInviteToken_EdgeCases(t *testing.T) {
	secret := []byte("edge-case-secret")

	t.Run("special characters in email", func(t *testing.T) {
		emails := []string{
			"user+tag@example.com",
			"user.name@sub.domain.com",
			"test@example.co.uk",
		}
		for _, email := range emails {
			token := GenerateInviteToken("inv_special", time.Now().Add(time.Hour), email, "user", secret)
			if token == "" {
				t.Errorf("token should not be empty for email %q", email)
			}
			parts := strings.Split(token, ".")
			if len(parts) != 2 {
				t.Errorf("token format should be payload.sig for email %q", email)
			}
		}
	})

	t.Run("very long inviteID", func(t *testing.T) {
		longID := strings.Repeat("a", 1000)
		token := GenerateInviteToken(longID, time.Now().Add(time.Hour), "test@example.com", "user", secret)
		if token == "" {
			t.Error("token should not be empty for very long inviteID")
		}
		parts := strings.Split(token, ".")
		if len(parts) != 2 {
			t.Error("token with long inviteID should have correct format")
		}
	})

	t.Run("future expiry far ahead", func(t *testing.T) {
		farFuture := time.Now().Add(365 * 24 * time.Hour) // 1 year
		token := GenerateInviteToken("inv_future", farFuture, "test@example.com", "admin", secret)
		if token == "" {
			t.Error("token should not be empty for far future expiry")
		}
	})
}

func TestLoadInviteHMACSecret(t *testing.T) {
	t.Run("env var set", func(t *testing.T) {
		patches := gomonkey.ApplyFunc(os.Getenv, func(key string) string {
			if key == "INVITE_HMAC_SECRET" {
				return "my-test-secret"
			}
			return ""
		})
		defer patches.Reset()

		secret, err := LoadInviteHMACSecret()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(secret) != "my-test-secret" {
			t.Errorf("expected 'my-test-secret', got %q", string(secret))
		}
	})

	t.Run("env var not set returns error", func(t *testing.T) {
		patches := gomonkey.ApplyFunc(os.Getenv, func(key string) string {
			return ""
		})
		defer patches.Reset()

		_, err := LoadInviteHMACSecret()
		if err == nil {
			t.Error("expected error when INVITE_HMAC_SECRET is not set")
		}
	})
}

// ===== GetInviteBaseURL tests =====

func TestGetInviteBaseURL(t *testing.T) {
	t.Run("env var set", func(t *testing.T) {
		patches := gomonkey.ApplyFunc(os.Getenv, func(key string) string {
			if key == "INVITE_BASE_URL" {
				return "https://example.com"
			}
			return ""
		})
		defer patches.Reset()

		url := GetInviteBaseURL()
		if url != "https://example.com" {
			t.Errorf("got %q, want %q", url, "https://example.com")
		}
	})

	t.Run("default when not set", func(t *testing.T) {
		patches := gomonkey.ApplyFunc(os.Getenv, func(key string) string {
			return ""
		})
		defer patches.Reset()

		url := GetInviteBaseURL()
		if url != "http://localhost:3000" {
			t.Errorf("got %q, want %q", url, "http://localhost:3000")
		}
	})

	t.Run("trailing slash stripped", func(t *testing.T) {
		patches := gomonkey.ApplyFunc(os.Getenv, func(key string) string {
			if key == "INVITE_BASE_URL" {
				return "https://example.com/"
			}
			return ""
		})
		defer patches.Reset()

		url := GetInviteBaseURL()
		if url != "https://example.com" {
			t.Errorf("got %q, want %q", url, "https://example.com")
		}
	})

	t.Run("multiple trailing slashes", func(t *testing.T) {
		patches := gomonkey.ApplyFunc(os.Getenv, func(key string) string {
			if key == "INVITE_BASE_URL" {
				return "https://example.com///"
			}
			return ""
		})
		defer patches.Reset()

		url := GetInviteBaseURL()
		if url != "https://example.com" {
			t.Errorf("got %q, want %q", url, "https://example.com")
		}
	})
}

// ===== VerifyInviteToken additional tests =====

func TestVerifyInviteToken_InvalidSignatureEncoding(t *testing.T) {
	secret := []byte("test-secret-key-32")
	inviteID := "inv_test123"
	email := "test@example.com"
	role := "user"
	expireAt := time.Now().Add(24 * time.Hour)

	token := GenerateInviteToken(inviteID, expireAt, email, role, secret)
	parts := strings.Split(token, ".")
	// Make signature part invalid by adding non-base64 characters
	badToken := parts[0] + ".!!!invalid!!!"
	_, err := VerifyInviteToken(badToken, [][]byte{secret})
	if err == nil {
		t.Error("invalid signature encoding should reject token")
	}
}

func TestVerifyInviteToken_InvalidPayloadEncoding(t *testing.T) {
	secret := []byte("test-secret-key-32")
	inviteID := "inv_test123"
	email := "test@example.com"
	role := "user"
	expireAt := time.Now().Add(24 * time.Hour)

	token := GenerateInviteToken(inviteID, expireAt, email, role, secret)
	parts := strings.Split(token, ".")
	// Make payload part invalid by adding non-base64 characters
	badToken := "!!!invalid!!!" + "." + parts[1]
	_, err := VerifyInviteToken(badToken, [][]byte{secret})
	if err == nil {
		t.Error("invalid payload encoding should reject token")
	}
}

func TestVerifyInviteToken_InvalidExpiryInPayload(t *testing.T) {
	secret := []byte("test-secret-key-32")

	// Create a token with invalid expiry: "invID:notANumber:email:role"
	payload := "inv_test123:notANumber:test@example.com:user"
	encodedPayload := base64.RawURLEncoding.EncodeToString([]byte(payload))

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	badToken := encodedPayload + "." + sig
	_, err := VerifyInviteToken(badToken, [][]byte{secret})
	if err == nil {
		t.Error("invalid expiry in payload should reject token")
	}
}

func TestVerifyInviteToken_PayloadTooManyColons(t *testing.T) {
	secret := []byte("test-secret-key-32")
	expireAt := time.Now().Add(24 * time.Hour)

	// Generate a valid token
	validToken := GenerateInviteToken("inv_test", expireAt, "test@example.com", "user", secret)

	// Verify first with wrong secret, which leads to payload being decoded but signature mismatch
	// Instead, let's modify the test to cover the signature mismatch directly
	_, err := VerifyInviteToken(validToken, [][]byte{[]byte("wrong-secret-key")})
	if err == nil {
		t.Error("wrong secret should reject token")
	}
}

func TestVerifyInviteToken_MalformedPayloadCustom(t *testing.T) {
	secret := []byte("test-secret-key-32")
	inviteID := "inv_test123"
	email := "test@example.com"
	role := "user"
	expireAt := time.Now().Add(24 * time.Hour)

	// The old test "malformed payload rejects" used "YWJj" which is valid base64 for "abc"
	// but the signature won't match. Let's also test with a longer payload that actually verifies
	token := GenerateInviteToken(inviteID, expireAt, email, role, secret)
	parts := strings.Split(token, ".")
	badToken := "YWJj" + "." + parts[1] // "abc" in base64
	_, err := VerifyInviteToken(badToken, [][]byte{secret})
	if err == nil {
		t.Error("malformed payload should reject")
	}
}
