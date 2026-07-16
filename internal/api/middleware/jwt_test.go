package middleware

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
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

// ── AuthMiddleware Tests ──

func TestAuthMiddleware_ValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := NewJWTManager("test-secret", time.Hour)

	patches := gomonkey.ApplyMethodReturn(m, "ValidateToken", &JWTClaims{
		UserID:   "user-1",
		Username: "testuser",
		Role:     "user",
	}, nil)
	defer patches.Reset()

	router := gin.New()
	router.Use(m.AuthMiddleware())
	router.GET("/test", func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		username, _ := c.Get("username")
		role, _ := c.Get("role")
		c.JSON(200, gin.H{
			"user_id":  userID,
			"username": username,
			"role":     role,
		})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer any-token-here")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_MissingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := NewJWTManager("test-secret", time.Hour)

	router := gin.New()
	router.Use(m.AuthMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_NotBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := NewJWTManager("test-secret", time.Hour)

	router := gin.New()
	router.Use(m.AuthMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic dGVzdDp0ZXN0")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for non-Bearer, got %d", w.Code)
	}
}

func TestAuthMiddleware_InvalidFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := NewJWTManager("test-secret", time.Hour)

	router := gin.New()
	router.Use(m.AuthMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Invalid")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := NewJWTManager("test-secret", time.Hour)

	patches := gomonkey.ApplyMethodReturn(m, "ValidateToken", (*JWTClaims)(nil), fmt.Errorf("invalid token"))
	defer patches.Reset()

	router := gin.New()
	router.Use(m.AuthMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ── Token Generation with Different Roles ──

func TestJWTManager_GenerateToken_Roles(t *testing.T) {
	m := NewJWTManager("test-jwt-secret", 24*time.Hour)

	roles := []string{"user", "admin", "system_admin"}
	for _, role := range roles {
		t.Run(role, func(t *testing.T) {
			token, err := m.GenerateToken("user-id", "testuser", role)
			if err != nil {
				t.Fatalf("GenerateToken(%s) error: %v", role, err)
			}
			if token == "" {
				t.Error("token should not be empty")
			}

			claims, err := m.ValidateToken(token)
			if err != nil {
				t.Fatalf("ValidateToken(%s) error: %v", role, err)
			}
			if claims.Role != role {
				t.Errorf("role: got %s, want %s", claims.Role, role)
			}
		})
	}
}

func TestJWTManager_ValidateToken_EmptyString(t *testing.T) {
	m := NewJWTManager("test-secret", time.Hour)
	_, err := m.ValidateToken("")
	if err == nil {
		t.Error("empty token should be rejected")
	}
}

func TestJWTManager_ValidateToken_Malformed(t *testing.T) {
	m := NewJWTManager("test-secret", time.Hour)
	_, err := m.ValidateToken("not-a-jwt-token-at-all")
	if err == nil {
		t.Error("malformed token should be rejected")
	}
}

func TestJWTManager_ValidateToken_WrongSecretPrefix(t *testing.T) {
	m := NewJWTManager("test-secret", time.Hour)
	// Generate with one secret, validate with different
	m2 := NewJWTManager("different-secret", time.Hour)
	token, _ := m.GenerateToken("user-1", "user", "user")

	_, err := m2.ValidateToken(token)
	if err == nil {
		t.Error("token signed with different secret should be rejected")
	}
}

// ── HashPassword & CheckPassword Edge Cases ──

func TestHashPassword_Empty(t *testing.T) {
	hash, err := HashPassword("")
	if err != nil {
		t.Fatalf("HashPassword empty: %v", err)
	}
	if hash == "" {
		t.Error("even empty password should produce a hash")
	}
}

func TestCheckPassword_EmptyHash(t *testing.T) {
	err := CheckPassword("", "anything")
	if err == nil {
		t.Error("empty hash should fail")
	}
}

// ── Coverage Gap Tests (error paths) ──

func TestHashPassword_Error(t *testing.T) {
	patches := gomonkey.ApplyFuncReturn(bcrypt.GenerateFromPassword, []byte{}, fmt.Errorf("bcrypt error"))
	defer patches.Reset()

	_, err := HashPassword("test")
	if err == nil {
		t.Error("expected error from HashPassword")
	}
}

func TestGenerateRandomPassword_Error(t *testing.T) {
	patches := gomonkey.ApplyFuncReturn(rand.Int, nil, fmt.Errorf("rand error"))
	defer patches.Reset()

	_, err := GenerateRandomPassword(16)
	if err == nil {
		t.Error("expected error from GenerateRandomPassword")
	}
}

func TestGenerateTokenBase64_Error(t *testing.T) {
	patches := gomonkey.ApplyFuncReturn(rand.Read, 0, fmt.Errorf("read error"))
	defer patches.Reset()

	_, err := GenerateTokenBase64(32)
	if err == nil {
		t.Error("expected error from GenerateTokenBase64")
	}
}

func TestGenerateShortID_Error(t *testing.T) {
	patches := gomonkey.ApplyFuncReturn(rand.Read, 0, fmt.Errorf("read error"))
	defer patches.Reset()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.GET("/test", func(c *gin.Context) {
		requestID, _ := c.Get("request_id")
		c.JSON(200, gin.H{"request_id": requestID})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 even when rand.Read fails, got %d", w.Code)
	}
	if got := w.Header().Get("X-Request-ID"); got != "unknown" {
		t.Errorf("X-Request-ID: got %q, want 'unknown' when rand.Read fails", got)
	}
}
