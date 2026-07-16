package middleware

import (
	"strings"
	"testing"
	"time"
)

func TestNewJWTManager(t *testing.T) {
	m := NewJWTManager("test-secret", 1*time.Hour)
	if m == nil {
		t.Fatal("NewJWTManager returned nil")
	}
	if m.GetExpiration() != 1*time.Hour {
		t.Errorf("GetExpiration: got %v, want 1h", m.GetExpiration())
	}
}

func TestJWTManager_GenerateAndValidateToken(t *testing.T) {
	m := NewJWTManager("test-jwt-secret", 24*time.Hour)

	userID := "user-123"
	username := "testuser"
	role := "admin"

	token, err := m.GenerateToken(userID, username, role)
	if err != nil {
		t.Fatalf("GenerateToken error: %v", err)
	}
	if token == "" {
		t.Fatal("token should not be empty")
	}

	t.Run("validate token", func(t *testing.T) {
		claims, err := m.ValidateToken(token)
		if err != nil {
			t.Fatalf("ValidateToken error: %v", err)
		}
		if claims.UserID != userID {
			t.Errorf("UserID: got %s, want %s", claims.UserID, userID)
		}
		if claims.Username != username {
			t.Errorf("Username: got %s, want %s", claims.Username, username)
		}
		if claims.Role != role {
			t.Errorf("Role: got %s, want %s", claims.Role, role)
		}
	})

	t.Run("wrong secret rejects", func(t *testing.T) {
		m2 := NewJWTManager("different-secret", 24*time.Hour)
		_, err := m2.ValidateToken(token)
		if err == nil {
			t.Error("token signed with different secret should be rejected")
		}
	})

	t.Run("tampered token rejects", func(t *testing.T) {
		tampered := token[:len(token)-2] + "AA"
		_, err := m.ValidateToken(tampered)
		if err == nil {
			t.Error("tampered token should be rejected")
		}
	})

	t.Run("expired token rejects", func(t *testing.T) {
		short := NewJWTManager("short-secret", 1*time.Millisecond)
		shortToken, _ := short.GenerateToken(userID, username, role)
		time.Sleep(10 * time.Millisecond)
		_, err := short.ValidateToken(shortToken)
		if err == nil {
			t.Error("expired token should be rejected")
		}
	})
}

func TestHashPassword(t *testing.T) {
	t.Run("generates non-empty hash", func(t *testing.T) {
		hash, err := HashPassword("mypassword")
		if err != nil {
			t.Fatalf("HashPassword error: %v", err)
		}
		if hash == "" {
			t.Error("hash should not be empty")
		}
	})

	t.Run("hash starts with bcrypt prefix", func(t *testing.T) {
		hash, _ := HashPassword("test")
		if !strings.HasPrefix(hash, "$2a$") {
			t.Errorf("hash should start with $2a$, got %s", hash)
		}
	})

	t.Run("different inputs produce different hashes", func(t *testing.T) {
		h1, _ := HashPassword("password1")
		h2, _ := HashPassword("password2")
		if h1 == h2 {
			t.Error("different passwords should produce different hashes")
		}
	})
}

func TestCheckPassword(t *testing.T) {
	hash, _ := HashPassword("correct-password")

	t.Run("correct password succeeds", func(t *testing.T) {
		if err := CheckPassword(hash, "correct-password"); err != nil {
			t.Errorf("correct password should succeed: %v", err)
		}
	})

	t.Run("wrong password fails", func(t *testing.T) {
		if err := CheckPassword(hash, "wrong-password"); err == nil {
			t.Error("wrong password should fail")
		}
	})

	t.Run("empty password", func(t *testing.T) {
		if err := CheckPassword(hash, ""); err == nil {
			t.Error("empty password should fail")
		}
	})
}

func TestGenerateRandomPassword(t *testing.T) {
	t.Run("correct length", func(t *testing.T) {
		pwd, err := GenerateRandomPassword(16)
		if err != nil {
			t.Fatalf("GenerateRandomPassword error: %v", err)
		}
		if len(pwd) != 16 {
			t.Errorf("password length: got %d, want 16", len(pwd))
		}
	})

	t.Run("unique values", func(t *testing.T) {
		p1, _ := GenerateRandomPassword(16)
		p2, _ := GenerateRandomPassword(16)
		if p1 == p2 {
			t.Error("successive calls should produce different passwords")
		}
	})
}

func TestSecureCompare(t *testing.T) {
	t.Run("equal strings", func(t *testing.T) {
		if !SecureCompare("abc", "abc") {
			t.Error("equal strings should return true")
		}
	})

	t.Run("different length", func(t *testing.T) {
		if SecureCompare("abc", "abcd") {
			t.Error("different length strings should return false")
		}
	})

	t.Run("different content", func(t *testing.T) {
		if SecureCompare("abc", "abd") {
			t.Error("different content should return false")
		}
	})

	t.Run("empty strings", func(t *testing.T) {
		if !SecureCompare("", "") {
			t.Error("empty strings should match")
		}
	})
}

func TestGenerateTokenBase64(t *testing.T) {
	t.Run("generates base64 string", func(t *testing.T) {
		token, err := GenerateTokenBase64(32)
		if err != nil {
			t.Fatalf("GenerateTokenBase64 error: %v", err)
		}
		if token == "" {
			t.Error("token should not be empty")
		}
	})

	t.Run("unique values", func(t *testing.T) {
		t1, _ := GenerateTokenBase64(32)
		t2, _ := GenerateTokenBase64(32)
		if t1 == t2 {
			t.Error("successive calls should produce different tokens")
		}
	})
}
