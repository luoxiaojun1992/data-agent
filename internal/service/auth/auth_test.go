package auth

import (
	"testing"
)

func TestNewService(t *testing.T) {
	s := NewService(nil, nil)
	if s == nil {
		t.Fatal("NewService() should not return nil")
	}
}

func TestLoginRequest_Defaults(t *testing.T) {
	req := LoginRequest{
		Username: "testuser",
		Password: "testpassword",
	}
	if req.Username != "testuser" {
		t.Errorf("Username = %q, want %q", req.Username, "testuser")
	}
	if req.Password != "testpassword" {
		t.Errorf("Password = %q, want %q", req.Password, "testpassword")
	}
}

func TestLoginResponse_Fields(t *testing.T) {
	resp := LoginResponse{
		UserID:      "user123",
		Username:    "john",
		Role:        "admin",
		AccessToken: "token123",
		TokenType:   "Bearer",
		ExpiresIn:   3600,
	}
	if resp.UserID != "user123" {
		t.Errorf("UserID = %q, want %q", resp.UserID, "user123")
	}
	if resp.TokenType != "Bearer" {
		t.Errorf("TokenType = %q, want %q", resp.TokenType, "Bearer")
	}
}

func TestRegisterRequest_Defaults(t *testing.T) {
	req := RegisterRequest{
		Username: "newuser",
		Password: "newpassword",
	}
	if req.Username != "newuser" {
		t.Errorf("Username = %q, want %q", req.Username, "newuser")
	}
}

func TestRegisterResponse_Fields(t *testing.T) {
	resp := RegisterResponse{
		UserID:   "user456",
		Username: "bob",
		Role:     "user",
		Message:  "Registration successful",
	}
	if resp.Message == "" {
		t.Error("Message should not be empty")
	}
	if resp.Role != "user" {
		t.Errorf("Role = %q, want %q", resp.Role, "user")
	}
}

func TestService_SetInviteRepo(t *testing.T) {
	s := NewService(nil, nil)
	s.SetInviteRepo(nil)
	// Should not panic with nil
	if s.inviteRepo != nil {
		t.Error("inviteRepo should be nil after SetInviteRepo(nil)")
	}
}

func TestService_SetHMACSecret(t *testing.T) {
	s := NewService(nil, nil)
	s.SetHMACSecret([]byte("test-secret"))
	if string(s.hmacSecret) != "test-secret" {
		t.Errorf("hmacSecret = %q, want %q", string(s.hmacSecret), "test-secret")
	}
}

func TestService_IsInviteEnabled(t *testing.T) {
	t.Run("not enabled by default", func(t *testing.T) {
		s := NewService(nil, nil)
		if s.IsInviteEnabled() {
			t.Error("IsInviteEnabled() should return false with no hmacSecret")
		}
	})

	t.Run("enabled when secret is set", func(t *testing.T) {
		s := NewService(nil, nil)
		s.SetHMACSecret([]byte("test-secret"))
		if !s.IsInviteEnabled() {
			t.Error("IsInviteEnabled() should return true when hmacSecret is set")
		}
	})

	t.Run("not enabled with empty secret", func(t *testing.T) {
		s := NewService(nil, nil)
		s.SetHMACSecret([]byte{})
		if s.IsInviteEnabled() {
			t.Error("IsInviteEnabled() should return false with empty hmacSecret")
		}
	})
}

func TestComputeTokenHash(t *testing.T) {
	t.Run("produces consistent hash", func(t *testing.T) {
		h1 := computeTokenHash("test-token")
		h2 := computeTokenHash("test-token")
		if h1 != h2 {
			t.Error("computeTokenHash should produce consistent results")
		}
	})

	t.Run("different tokens produce different hashes", func(t *testing.T) {
		h1 := computeTokenHash("token-a")
		h2 := computeTokenHash("token-b")
		if h1 == h2 {
			t.Error("different tokens should produce different hashes")
		}
	})

	t.Run("non-empty output", func(t *testing.T) {
		h := computeTokenHash("something")
		if h == "" {
			t.Error("computeTokenHash should return non-empty hash")
		}
	})
}

func TestCreateInviteRequest_Defaults(t *testing.T) {
	req := CreateInviteRequest{
		Email: "user@example.com",
		Role:  "user",
	}
	if req.Email != "user@example.com" {
		t.Errorf("Email = %q, want %q", req.Email, "user@example.com")
	}
}

func TestCreateInviteResponse_Fields(t *testing.T) {
	resp := CreateInviteResponse{
		InviteID:  "inv_abc123",
		InviteURL: "https://example.com/register?token=abc",
	}
	if resp.InviteID == "" {
		t.Error("InviteID should not be empty")
	}
	if resp.InviteURL == "" {
		t.Error("InviteURL should not be empty")
	}
}
