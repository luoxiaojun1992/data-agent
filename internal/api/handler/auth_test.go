package handler

import (
	"fmt"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	authsvc "github.com/luoxiaojun1992/data-agent/internal/service/auth"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func createTestAuthService() *authsvc.Service {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	return authsvc.NewService(nil, jwt)
}

func TestAuthHandler_Login(t *testing.T) {
	svc := createTestAuthService()
	h := NewAuthHandler(svc)

	t.Run("valid login", func(t *testing.T) {
		patches := gomonkey.ApplyMethodReturn(svc, "Login", &authsvc.LoginResponse{
			UserID:      "user-123",
			Username:    "testuser",
			Role:        "user",
			AccessToken: "jwt-token",
			TokenType:   "Bearer",
			ExpiresIn:   86400,
		}, nil)
		defer patches.Reset()

		body := `{"username":"testuser","password":"password"}`
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		h.Login(c)
		if w.Code != http.StatusOK {
			t.Errorf("status: got %d, want 200", w.Code)
		}
	})

	t.Run("invalid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/auth/login", strings.NewReader(`{"bad`))
		c.Request.Header.Set("Content-Type", "application/json")

		h.Login(c)
		if w.Code != http.StatusBadRequest {
			t.Errorf("status: got %d, want 400", w.Code)
		}
	})

	t.Run("login error", func(t *testing.T) {
		patches := gomonkey.ApplyMethodReturn(svc, "Login", (*authsvc.LoginResponse)(nil), fmt.Errorf("auth failed"))
		defer patches.Reset()

		body := `{"username":"test","password":"wrongpass"}`
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		h.Login(c)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("status: got %d, want 401, body: %s", w.Code, w.Body.String())
		}
	})
}

func TestAuthHandler_Register(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := authsvc.NewService(nil, jwt)
	h := NewAuthHandler(svc)

	t.Run("register when invite disabled", func(t *testing.T) {
		patches := gomonkey.ApplyMethodReturn(svc, "Register", &authsvc.RegisterResponse{
			Username: "newuser",
			Role:     "user",
			Message:  "ok",
		}, nil)
		defer patches.Reset()

		body := `{"username":"newuser","password":"Pass123!"}`
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/auth/register", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		h.Register(c)
		if w.Code != http.StatusCreated {
			t.Errorf("register status: got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("register when invite enabled", func(t *testing.T) {
		svc.SetHMACSecret([]byte("secret-key-at-least-16-chars"))
		body := `{"username":"newuser","password":"Pass123!"}`
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/auth/register", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		h.Register(c)
		if w.Code != http.StatusGone {
			t.Errorf("Register with invite enabled: got %d, want 410", w.Code)
		}
		svc.SetHMACSecret(nil) // reset
	})
}

func TestAuthHandler_RefreshToken(t *testing.T) {
	svc := createTestAuthService()
	h := NewAuthHandler(svc)

	patches := gomonkey.ApplyMethodReturn(svc, "RefreshToken", &authsvc.LoginResponse{
		AccessToken: "refreshed",
		TokenType:   "Bearer",
	}, nil)
	defer patches.Reset()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/refresh", nil)
	c.Set("user_id", "user-1")
	c.Set("username", "test")
	c.Set("role", "user")

	h.RefreshToken(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
}

func TestAuthHandler_GetProfile(t *testing.T) {
	h := NewAuthHandler(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/auth/profile", nil)
	c.Set("user_id", "user-1")
	c.Set("username", "testuser")
	c.Set("role", "admin")

	h.GetProfile(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["username"] != "testuser" {
		t.Errorf("username: got %v", resp["username"])
	}
}

func TestAuthHandler_CreateInvite(t *testing.T) {
	svc := createTestAuthService()
	svc.SetInviteRepo(nil)
	svc.SetHMACSecret([]byte("secret-key-for-testing-purposes"))
	h := NewAuthHandler(svc)

	t.Run("admin can create invite", func(t *testing.T) {
		patches := gomonkey.ApplyMethodReturn(svc, "CreateInvite", &authsvc.CreateInviteResponse{
			InviteID:  "inv_test",
			InviteURL: "http://localhost:3000/register?token=xxx",
		}, nil)
		defer patches.Reset()

		body := `{"email":"test@example.com","role":"user","expire_hours":24}`
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/admin/invites", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user_id", "admin-1")
		c.Set("role", "system_admin")

		h.CreateInvite(c)
		if w.Code != http.StatusCreated {
			t.Errorf("status: got %d, want 201: %s", w.Code, w.Body.String())
		}
	})

	t.Run("admin cannot invite admin role", func(t *testing.T) {
		body := `{"email":"test@example.com","role":"admin"}`
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/admin/invites", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user_id", "admin-1")
		c.Set("role", "admin")

		h.CreateInvite(c)
		if w.Code != http.StatusForbidden {
			t.Errorf("admin creating admin invite: got %d, want 403", w.Code)
		}
	})
}

func TestAuthHandler_CompleteRegistration(t *testing.T) {
	svc := createTestAuthService()
	h := NewAuthHandler(svc)

	t.Run("valid registration", func(t *testing.T) {
		patches := gomonkey.ApplyMethodReturn(svc, "CompleteRegistration", &authsvc.CompleteRegistrationResponse{
			UserID:      primitive.NewObjectID().Hex(),
			Username:    "newuser",
			Role:        "user",
			AccessToken: "jwt",
			TokenType:   "Bearer",
		}, nil)
		defer patches.Reset()

		body := `{"token":"valid","username":"newuser","password":"Pass123!","display_name":"New"}`
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/auth/complete-registration", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		h.CompleteRegistration(c)
		if w.Code != http.StatusCreated {
			t.Errorf("status: got %d, want 201: %s", w.Code, w.Body.String())
		}
	})
}
