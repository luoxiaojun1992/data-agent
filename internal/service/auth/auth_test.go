package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
	"github.com/luoxiaojun1992/data-agent/internal/logic"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestIsInviteEnabled(t *testing.T) {
	s := &Service{}
	if s.IsInviteEnabled() {
		t.Error("hmacSecret should be false by default")
	}
	s.hmacSecret = []byte("test")
	if !s.IsInviteEnabled() {
		t.Error("hmacSecret should be true after setting")
	}
}

func TestLogin_Success(t *testing.T) {
	repo := &mongo.UserRepository{}
	user := &model.User{
		ID:           primitive.NewObjectID(),
		Username:     "testuser",
		PasswordHash: "$2a$10$dummy",
		Role:         model.RoleUser,
		Status:       model.StatusEnabled,
	}

	patches := gomonkey.ApplyMethodReturn(repo, "FindByUsername", user, nil)
	defer patches.Reset()
	patches.ApplyFunc(middleware.CheckPassword, func(hash, pw string) error { return nil })
	patches.ApplyMethodReturn(&middleware.JWTManager{}, "GenerateToken", "valid-token", nil)

	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(repo, jwt)

	resp, err := svc.Login(context.Background(), &LoginRequest{Username: "testuser", Password: "pass"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if resp.AccessToken != "valid-token" {
		t.Errorf("token: got %s", resp.AccessToken)
	}
}

func TestLogin_NotFound(t *testing.T) {
	repo := &mongo.UserRepository{}
	patches := gomonkey.ApplyMethodReturn(repo, "FindByUsername", (*model.User)(nil), nil)
	defer patches.Reset()

	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(repo, jwt)
	_, err := svc.Login(context.Background(), &LoginRequest{Username: "nobody", Password: "pass"})
	if err == nil {
		t.Error("should error for nonexistent user")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	repo := &mongo.UserRepository{}
	user := &model.User{Username: "testuser", PasswordHash: "hash", Role: model.RoleUser}
	patches := gomonkey.ApplyMethodReturn(repo, "FindByUsername", user, nil)
	defer patches.Reset()
	patches.ApplyFunc(middleware.CheckPassword, func(hash, pw string) error { return context.DeadlineExceeded })

	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(repo, jwt)
	_, err := svc.Login(context.Background(), &LoginRequest{Username: "testuser", Password: "wrong"})
	if err == nil {
		t.Error("should error for wrong password")
	}
}

func TestRegister_Success(t *testing.T) {
	repo := &mongo.UserRepository{}
	patches := gomonkey.ApplyMethodReturn(repo, "FindByUsername", (*model.User)(nil), nil)
	defer patches.Reset()
	patches.ApplyFunc(middleware.HashPassword, func(pw string) (string, error) { return "$2a$hashed", nil })
	patches.ApplyMethodReturn(repo, "Create", nil)

	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(repo, jwt)

	resp, err := svc.Register(context.Background(), &RegisterRequest{Username: "newuser", Password: "Pass123!", Role: model.RoleUser})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if resp.Username != "newuser" {
		t.Errorf("username: got %s", resp.Username)
	}
}

func TestRegister_Duplicate(t *testing.T) {
	repo := &mongo.UserRepository{}
	existing := &model.User{Username: "exists"}
	patches := gomonkey.ApplyMethodReturn(repo, "FindByUsername", existing, nil)
	defer patches.Reset()

	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(repo, jwt)
	_, err := svc.Register(context.Background(), &RegisterRequest{Username: "exists", Password: "Pass123!"})
	if err == nil {
		t.Error("should error for duplicate")
	}
}

func TestRefreshToken(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	patches := gomonkey.ApplyMethodReturn(jwt, "GenerateToken", "refreshed-token", nil)
	defer patches.Reset()

	resp, err := svc.RefreshToken(context.Background(), "uid", "uname", "user")
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if resp.AccessToken != "refreshed-token" {
		t.Errorf("token: got %s", resp.AccessToken)
	}
}

func TestSetInviteRepo(t *testing.T) {
	svc := &Service{}
	svc.SetInviteRepo(nil)
	if svc.inviteRepo != nil {
		t.Error("should be nil")
	}
}

func TestSetHMACSecret(t *testing.T) {
	svc := &Service{}
	svc.SetHMACSecret([]byte("my-secret"))
	if string(svc.hmacSecret) != "my-secret" {
		t.Error("should store secret")
	}
}

// ── Invite tests ──

func TestCreateInvite(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	invRepo := &mongo.InviteRepository{}
	svc.SetInviteRepo(invRepo)
	svc.SetHMACSecret([]byte("test-secret-key-at-least-16"))

	patches := gomonkey.ApplyMethodReturn(invRepo, "Create", nil)
	defer patches.Reset()

	resp, err := svc.CreateInvite(context.Background(), "admin-1", &CreateInviteRequest{
		Email:       "test@example.com",
		Role:        "user",
		ExpireHours: 24,
	})
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if resp.InviteID == "" {
		t.Error("InviteID should not be empty")
	}
	if resp.InviteURL == "" {
		t.Error("InviteURL should not be empty")
	}
}

func TestCreateInvite_NoRepo(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	svc.SetHMACSecret([]byte("test-secret-key"))

	_, err := svc.CreateInvite(context.Background(), "admin-1", &CreateInviteRequest{})
	if err == nil {
		t.Error("should error without repo")
	}
}

func TestCreateInvite_SystemAdminBlocked(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	invRepo := &mongo.InviteRepository{}
	svc.SetInviteRepo(invRepo)
	svc.SetHMACSecret([]byte("test-secret-key-16chars"))

	_, err := svc.CreateInvite(context.Background(), "admin-1", &CreateInviteRequest{
		Role: "system_admin",
	})
	if err == nil {
		t.Error("should error for system_admin role invite")
	}
}

func TestListInvites(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	invRepo := &mongo.InviteRepository{}
	svc.SetInviteRepo(invRepo)

	patches := gomonkey.ApplyMethodReturn(invRepo, "List", []model.Invite{}, int64(0), nil)
	defer patches.Reset()

	resp, err := svc.ListInvites(context.Background(), "", 1, 10)
	if err != nil {
		t.Fatalf("ListInvites: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("Total: got %d", resp.Total)
	}
}

func TestRevokeInvite(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	invRepo := &mongo.InviteRepository{}
	svc.SetInviteRepo(invRepo)

	patches := gomonkey.ApplyMethodReturn(invRepo, "Revoke", nil)
	defer patches.Reset()

	err := svc.RevokeInvite(context.Background(), "inv-1")
	if err != nil {
		t.Fatalf("RevokeInvite: %v", err)
	}
}

func TestUpdateHMACSecret(t *testing.T) {
	svc := &Service{}
	err := svc.UpdateHMACSecret(context.Background(), "new-secret-16chars")
	if err != nil {
		t.Fatalf("UpdateHMACSecret: %v", err)
	}
	if string(svc.hmacSecret) != "new-secret-16chars" {
		t.Error("should update secret")
	}
}

func TestUpdateHMACSecret_TooShort(t *testing.T) {
	svc := &Service{}
	err := svc.UpdateHMACSecret(context.Background(), "short")
	if err == nil {
		t.Error("should error for short secret")
	}
}

func TestCompleteRegistration(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	repo := &mongo.UserRepository{}
	svc.userRepo = repo
	invRepo := &mongo.InviteRepository{}
	svc.SetInviteRepo(invRepo)
	svc.SetHMACSecret([]byte("test-secret-key-for-complete-reg"))

	patches := gomonkey.ApplyMethodReturn(repo, "FindByUsername", (*model.User)(nil), nil)
	defer patches.Reset()
	patches.ApplyFunc(middleware.HashPassword, func(pw string) (string, error) { return "$2a$hashed", nil })
	patches.ApplyMethodReturn(repo, "Create", nil)
	patches.ApplyMethodReturn(invRepo, "MarkAccepted", nil)
	patches.ApplyMethodReturn(jwt, "GenerateToken", "jwt-token", nil)
	patches.ApplyMethodReturn(invRepo, "FindByInviteID", &model.Invite{
		InviteID:  "inv_test1",
		Status:    model.InviteStatusPending,
		Role:      "user",
		CreatedBy: "admin-1",
	}, nil)
	patches.ApplyFunc(logic.VerifyInviteToken, func(token string, secrets [][]byte) (*logic.InviteTokenPayload, error) {
		return &logic.InviteTokenPayload{InviteID: "inv_test1", Email: "test@example.com", Role: "user", ExpireAt: time.Now().Add(24 * time.Hour).Unix()}, nil
	})

	resp, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token:       "bW9jay10b2tlbg==.mocktoken",
		Username:    "newuser",
		Password:    "Pass123!",
		DisplayName: "New User",
	})
	if err != nil {
		t.Fatalf("CompleteRegistration: %v", err)
	}
	if resp.Username != "newuser" {
		t.Errorf("username: got %s", resp.Username)
	}
}

func TestVerifyInviteToken(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	invRepo := &mongo.InviteRepository{}
	svc.SetInviteRepo(invRepo)
	svc.SetHMACSecret([]byte("test-secret-key-for-verify"))

	t.Run("invalid token", func(t *testing.T) {
		resp, err := svc.VerifyInviteToken(context.Background(), "bad-token")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Valid {
			t.Error("invalid token should not be valid")
		}
	})

	t.Run("no repo", func(t *testing.T) {
		svc2 := &Service{hmacSecret: []byte("test")}
		_, err := svc2.VerifyInviteToken(context.Background(), "any")
		if err == nil {
			t.Error("should error without repo")
		}
	})

	t.Run("valid pending invite", func(t *testing.T) {
		patches := gomonkey.ApplyFunc(logic.VerifyInviteToken, func(token string, secrets [][]byte) (*logic.InviteTokenPayload, error) {
			return &logic.InviteTokenPayload{
				InviteID: "inv_test1",
				Email:    "test@example.com",
				Role:     "user",
				ExpireAt: time.Now().Add(24 * time.Hour).Unix(),
			}, nil
		})
		defer patches.Reset()

		patches = patches.ApplyMethodFunc(invRepo, "FindByInviteID", func(ctx context.Context, inviteID string) (*model.Invite, error) {
			return &model.Invite{
				InviteID: "inv_test1",
				Status:   model.InviteStatusPending,
				Role:     "user",
				Email:    "test@example.com",
			}, nil
		})

		resp, err := svc.VerifyInviteToken(context.Background(), "valid-token")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !resp.Valid {
			t.Error("token should be valid")
		}
		if resp.Email != "test@example.com" {
			t.Errorf("Email = %q", resp.Email)
		}
		if resp.Role != "user" {
			t.Errorf("Role = %q", resp.Role)
		}
	})

	t.Run("expired token", func(t *testing.T) {
		patches := gomonkey.ApplyFunc(logic.VerifyInviteToken, func(token string, secrets [][]byte) (*logic.InviteTokenPayload, error) {
			return &logic.InviteTokenPayload{
				InviteID: "inv_expired",
				Email:    "test@example.com",
				Role:     "user",
				ExpireAt: time.Now().Add(-1 * time.Hour).Unix(),
			}, nil
		})
		defer patches.Reset()

		resp, err := svc.VerifyInviteToken(context.Background(), "expired-token")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Valid {
			t.Error("expired token should not be valid")
		}
	})

	t.Run("already used invite", func(t *testing.T) {
		patches := gomonkey.ApplyFunc(logic.VerifyInviteToken, func(token string, secrets [][]byte) (*logic.InviteTokenPayload, error) {
			return &logic.InviteTokenPayload{
				InviteID: "inv_used",
				Email:    "test@example.com",
				Role:     "user",
				ExpireAt: time.Now().Add(24 * time.Hour).Unix(),
			}, nil
		})
		defer patches.Reset()

		patches = patches.ApplyMethodFunc(invRepo, "FindByInviteID", func(ctx context.Context, inviteID string) (*model.Invite, error) {
			return &model.Invite{
				InviteID: "inv_used",
				Status:   model.InviteStatusAccepted,
			}, nil
		})

		resp, err := svc.VerifyInviteToken(context.Background(), "used-token")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Valid {
			t.Error("already used invite should not be valid")
		}
	})

	t.Run("invite not found in DB", func(t *testing.T) {
		patches := gomonkey.ApplyFunc(logic.VerifyInviteToken, func(token string, secrets [][]byte) (*logic.InviteTokenPayload, error) {
			return &logic.InviteTokenPayload{
				InviteID: "inv_gone",
				Email:    "test@example.com",
				Role:     "user",
				ExpireAt: time.Now().Add(24 * time.Hour).Unix(),
			}, nil
		})
		defer patches.Reset()

		patches = patches.ApplyMethodFunc(invRepo, "FindByInviteID", func(ctx context.Context, inviteID string) (*model.Invite, error) {
			return nil, nil
		})

		resp, err := svc.VerifyInviteToken(context.Background(), "missing-token")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Valid {
			t.Error("invite not found should not be valid")
		}
	})

	t.Run("no hmac secret", func(t *testing.T) {
		svc2 := &Service{}
		svc2.SetInviteRepo(invRepo)
		_, err := svc2.VerifyInviteToken(context.Background(), "any-token")
		if err == nil {
			t.Error("should error without HMAC secret")
		}
	})
}

// ===== CompleteRegistration additional tests =====

func TestCompleteRegistration_UsernameConflict(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	repo := &mongo.UserRepository{}
	svc.userRepo = repo
	invRepo := &mongo.InviteRepository{}
	svc.SetInviteRepo(invRepo)
	svc.SetHMACSecret([]byte("test-secret-key-for-complete"))

	patches := gomonkey.ApplyFunc(logic.VerifyInviteToken, func(token string, secrets [][]byte) (*logic.InviteTokenPayload, error) {
		return &logic.InviteTokenPayload{
			InviteID: "inv_test1", Email: "test@example.com", Role: "user",
			ExpireAt: time.Now().Add(24 * time.Hour).Unix(),
		}, nil
	})
	defer patches.Reset()

	patches = patches.ApplyMethodFunc(invRepo, "FindByInviteID", func(ctx context.Context, inviteID string) (*model.Invite, error) {
		return &model.Invite{InviteID: "inv_test1", Status: model.InviteStatusPending, Role: "user", CreatedBy: "admin-1"}, nil
	})

	patches = patches.ApplyMethodFunc(repo, "FindByUsername", func(ctx context.Context, username string) (*model.User, error) {
		return &model.User{Username: "existinguser"}, nil
	})

	_, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "valid-token", Username: "existinguser", Password: "Pass123!", DisplayName: "New User",
	})
	if err == nil {
		t.Fatal("expected username conflict error, got nil")
	}
}

func TestCompleteRegistration_ExpiredToken(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	invRepo := &mongo.InviteRepository{}
	svc.SetInviteRepo(invRepo)
	svc.SetHMACSecret([]byte("test-secret-key-for-complete"))

	patches := gomonkey.ApplyFunc(logic.VerifyInviteToken, func(token string, secrets [][]byte) (*logic.InviteTokenPayload, error) {
		return &logic.InviteTokenPayload{
			InviteID: "inv_expired", Email: "test@example.com", Role: "user",
			ExpireAt: time.Now().Add(-1 * time.Hour).Unix(),
		}, nil
	})
	defer patches.Reset()

	_, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "expired-token", Username: "newuser", Password: "Pass123!", DisplayName: "New User",
	})
	if err == nil {
		t.Fatal("expected expired token error, got nil")
	}
}

func TestCompleteRegistration_InviteNotPending(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	repo := &mongo.UserRepository{}
	svc.userRepo = repo
	invRepo := &mongo.InviteRepository{}
	svc.SetInviteRepo(invRepo)
	svc.SetHMACSecret([]byte("test-secret-key-for-complete"))

	patches := gomonkey.ApplyFunc(logic.VerifyInviteToken, func(token string, secrets [][]byte) (*logic.InviteTokenPayload, error) {
		return &logic.InviteTokenPayload{
			InviteID: "inv_used", Email: "test@example.com", Role: "user",
			ExpireAt: time.Now().Add(24 * time.Hour).Unix(),
		}, nil
	})
	defer patches.Reset()

	patches = patches.ApplyMethodFunc(invRepo, "FindByInviteID", func(ctx context.Context, inviteID string) (*model.Invite, error) {
		return &model.Invite{InviteID: "inv_used", Status: model.InviteStatusRevoked, CreatedBy: "admin-1"}, nil
	})

	_, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "used-token", Username: "newuser", Password: "Pass123!", DisplayName: "New User",
	})
	if err == nil {
		t.Fatal("expected error for revoked invite, got nil")
	}
}

func TestCompleteRegistration_FindUserError(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	repo := &mongo.UserRepository{}
	svc.userRepo = repo
	invRepo := &mongo.InviteRepository{}
	svc.SetInviteRepo(invRepo)
	svc.SetHMACSecret([]byte("test-secret-key-for-complete"))

	patches := gomonkey.ApplyFunc(logic.VerifyInviteToken, func(token string, secrets [][]byte) (*logic.InviteTokenPayload, error) {
		return &logic.InviteTokenPayload{
			InviteID: "inv_test1", Email: "test@example.com", Role: "user",
			ExpireAt: time.Now().Add(24 * time.Hour).Unix(),
		}, nil
	})
	defer patches.Reset()

	patches = patches.ApplyMethodFunc(invRepo, "FindByInviteID", func(ctx context.Context, inviteID string) (*model.Invite, error) {
		return &model.Invite{InviteID: "inv_test1", Status: model.InviteStatusPending, Role: "user", CreatedBy: "admin-1"}, nil
	})

	patches = patches.ApplyMethodFunc(repo, "FindByUsername", func(ctx context.Context, username string) (*model.User, error) {
		return nil, errors.New("db error")
	})

	_, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "valid-token", Username: "newuser", Password: "Pass123!", DisplayName: "New User",
	})
	if err == nil {
		t.Fatal("expected error from FindByUsername, got nil")
	}
}

// ===== Login additional tests =====

func TestLogin_FindUserError(t *testing.T) {
	repo := &mongo.UserRepository{}
	patches := gomonkey.ApplyMethodFunc(repo, "FindByUsername", func(ctx context.Context, username string) (*model.User, error) {
		return nil, errors.New("database connection lost")
	})
	defer patches.Reset()

	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(repo, jwt)

	_, err := svc.Login(context.Background(), &LoginRequest{Username: "testuser", Password: "pass"})
	if err == nil {
		t.Error("should error for database error")
	}
}

// ===== Register additional tests =====

func TestRegister_FindUserError(t *testing.T) {
	repo := &mongo.UserRepository{}
	patches := gomonkey.ApplyMethodFunc(repo, "FindByUsername", func(ctx context.Context, username string) (*model.User, error) {
		return nil, errors.New("database error")
	})
	defer patches.Reset()

	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(repo, jwt)

	_, err := svc.Register(context.Background(), &RegisterRequest{Username: "newuser", Password: "Pass123!"})
	if err == nil {
		t.Error("should error for database error")
	}
}

func TestRegister_DefaultRole(t *testing.T) {
	repo := &mongo.UserRepository{}
	patches := gomonkey.ApplyMethodReturn(repo, "FindByUsername", (*model.User)(nil), nil)
	defer patches.Reset()
	patches.ApplyFunc(middleware.HashPassword, func(pw string) (string, error) { return "$2a$hashed", nil })
	patches.ApplyMethodReturn(repo, "Create", nil)

	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(repo, jwt)

	resp, err := svc.Register(context.Background(), &RegisterRequest{Username: "newuser2", Password: "Pass123!"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if resp.Role != "user" {
		t.Errorf("role should default to 'user', got %q", resp.Role)
	}
}

func TestRegister_HashPasswordError(t *testing.T) {
	repo := &mongo.UserRepository{}
	patches := gomonkey.ApplyMethodReturn(repo, "FindByUsername", (*model.User)(nil), nil)
	defer patches.Reset()
	patches.ApplyFunc(middleware.HashPassword, func(pw string) (string, error) {
		return "", errors.New("hash failed")
	})

	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(repo, jwt)

	_, err := svc.Register(context.Background(), &RegisterRequest{Username: "newuser", Password: "Pass123!"})
	if err == nil {
		t.Error("should error for hash failure")
	}
}

func TestRegister_AdminRole(t *testing.T) {
	repo := &mongo.UserRepository{}
	patches := gomonkey.ApplyMethodReturn(repo, "FindByUsername", (*model.User)(nil), nil)
	defer patches.Reset()
	patches.ApplyFunc(middleware.HashPassword, func(pw string) (string, error) { return "$2a$hashed", nil })
	patches.ApplyMethodReturn(repo, "Create", nil)

	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(repo, jwt)

	resp, err := svc.Register(context.Background(), &RegisterRequest{Username: "admin2", Password: "Pass123!", Role: model.RoleAdmin})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if resp.Role != "admin" {
		t.Errorf("role should be 'admin', got %q", resp.Role)
	}
}

// ===== RefreshToken additional test =====

func TestRefreshToken_GenerateError(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	patches := gomonkey.ApplyMethodReturn(jwt, "GenerateToken", "", errors.New("token generation failed"))
	defer patches.Reset()

	_, err := svc.RefreshToken(context.Background(), "uid", "uname", "user")
	if err == nil {
		t.Error("should error for token generation failure")
	}
}

// ===== CreateInvite additional tests =====

func TestCreateInvite_NoHMACSecret(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	invRepo := &mongo.InviteRepository{}
	svc.SetInviteRepo(invRepo)

	_, err := svc.CreateInvite(context.Background(), "admin-1", &CreateInviteRequest{Email: "test@example.com", Role: "user"})
	if err == nil {
		t.Error("should error without HMAC secret")
	}
}

func TestCreateInvite_DefaultExpireHours(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	invRepo := &mongo.InviteRepository{}
	svc.SetInviteRepo(invRepo)
	svc.SetHMACSecret([]byte("test-secret-key-16chars"))

	patches := gomonkey.ApplyMethodReturn(invRepo, "Create", nil)
	defer patches.Reset()

	resp, err := svc.CreateInvite(context.Background(), "admin-1", &CreateInviteRequest{
		Email: "test@example.com", Role: "user", ExpireHours: 0,
	})
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if resp.InviteID == "" {
		t.Error("InviteID should not be empty")
	}
}

func TestCreateInvite_DefaultRole(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	invRepo := &mongo.InviteRepository{}
	svc.SetInviteRepo(invRepo)
	svc.SetHMACSecret([]byte("test-secret-key-16chars"))

	patches := gomonkey.ApplyMethodReturn(invRepo, "Create", nil)
	defer patches.Reset()

	resp, err := svc.CreateInvite(context.Background(), "admin-1", &CreateInviteRequest{
		Email: "test@example.com", Role: "", ExpireHours: 24,
	})
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if resp.InviteID == "" {
		t.Error("InviteID should not be empty")
	}
}

// ===== ListInvites additional tests =====

func TestListInvites_NoRepo(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)

	_, err := svc.ListInvites(context.Background(), "", 1, 10)
	if err == nil {
		t.Error("should error without repo")
	}
}

func TestListInvites_DefaultPagination(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	invRepo := &mongo.InviteRepository{}
	svc.SetInviteRepo(invRepo)

	patches := gomonkey.ApplyMethodFunc(invRepo, "List", func(ctx context.Context, createdBy string, skip, limit int64) ([]model.Invite, int64, error) {
		return []model.Invite{
			{InviteID: "inv_1", Status: model.InviteStatusPending, Role: "user", CreatedBy: "admin-1"},
		}, int64(1), nil
	})
	defer patches.Reset()

	resp, err := svc.ListInvites(context.Background(), "", 0, 0)
	if err != nil {
		t.Fatalf("ListInvites: %v", err)
	}
	if resp.Total != 1 {
		t.Errorf("Total = %d, want 1", resp.Total)
	}
	if resp.Page != 1 {
		t.Errorf("Page = %d, want 1 (default)", resp.Page)
	}
	if resp.Size != 20 {
		t.Errorf("Size = %d, want 20 (default)", resp.Size)
	}
}

func TestListInvites_WithResults(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	invRepo := &mongo.InviteRepository{}
	svc.SetInviteRepo(invRepo)

	now := time.Now()
	patches := gomonkey.ApplyMethodFunc(invRepo, "List", func(ctx context.Context, createdBy string, skip, limit int64) ([]model.Invite, int64, error) {
		return []model.Invite{
			{InviteID: "inv_1", Email: "a@test.com", Role: "user", Status: model.InviteStatusPending, CreatedBy: "admin", CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour)},
			{InviteID: "inv_2", Email: "b@test.com", Role: "admin", Status: model.InviteStatusAccepted, CreatedBy: "admin", CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour)},
		}, int64(2), nil
	})
	defer patches.Reset()

	resp, err := svc.ListInvites(context.Background(), "", 1, 10)
	if err != nil {
		t.Fatalf("ListInvites: %v", err)
	}
	if len(resp.Invites) != 2 {
		t.Fatalf("got %d invites, want 2", len(resp.Invites))
	}
	if resp.Invites[0].InviteID != "inv_1" {
		t.Errorf("Invites[0].InviteID = %q", resp.Invites[0].InviteID)
	}
	if resp.Invites[1].Role != "admin" {
		t.Errorf("Invites[1].Role = %q", resp.Invites[1].Role)
	}
}

// ===== RevokeInvite additional test =====

func TestRevokeInvite_NoRepo(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)

	err := svc.RevokeInvite(context.Background(), "inv-1")
	if err == nil {
		t.Error("should error without repo")
	}
}

// ===== ComputeTokenHash test =====

func TestComputeTokenHash(t *testing.T) {
	h1 := computeTokenHash("test-token-123")
	if h1 == "" {
		t.Error("hash should not be empty")
	}

	h2 := computeTokenHash("test-token-123")
	if h1 != h2 {
		t.Error("same input should produce same hash")
	}

	h3 := computeTokenHash("different-token")
	if h1 == h3 {
		t.Error("different inputs should produce different hashes")
	}
}

// ===== CompleteRegistration: UserRepo.Create error =====

func TestCompleteRegistration_CreateUserError(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	repo := &mongo.UserRepository{}
	svc.userRepo = repo
	invRepo := &mongo.InviteRepository{}
	svc.SetInviteRepo(invRepo)
	svc.SetHMACSecret([]byte("test-secret-key-for-complete"))

	patches := gomonkey.ApplyFunc(logic.VerifyInviteToken, func(token string, secrets [][]byte) (*logic.InviteTokenPayload, error) {
		return &logic.InviteTokenPayload{
			InviteID: "inv_test1", Email: "test@example.com", Role: "user",
			ExpireAt: time.Now().Add(24 * time.Hour).Unix(),
		}, nil
	})
	defer patches.Reset()

	patches = patches.ApplyMethodFunc(invRepo, "FindByInviteID", func(ctx context.Context, inviteID string) (*model.Invite, error) {
		return &model.Invite{InviteID: "inv_test1", Status: model.InviteStatusPending, Role: "user", CreatedBy: "admin-1"}, nil
	})

	patches = patches.ApplyMethodFunc(repo, "FindByUsername", func(ctx context.Context, username string) (*model.User, error) {
		return nil, nil
	})

	patches.ApplyFunc(middleware.HashPassword, func(pw string) (string, error) { return "$2a$hashed", nil })

	patches = patches.ApplyMethodFunc(repo, "Create", func(ctx context.Context, user *model.User) error {
		return errors.New("create user failed")
	})

	_, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "valid-token", Username: "newuser", Password: "Pass123!", DisplayName: "New User",
	})
	if err == nil {
		t.Fatal("expected error from Create user, got nil")
	}
}

// ===== CompleteRegistration: GenerateToken error =====

func TestCompleteRegistration_GenerateTokenError(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	repo := &mongo.UserRepository{}
	svc.userRepo = repo
	invRepo := &mongo.InviteRepository{}
	svc.SetInviteRepo(invRepo)
	svc.SetHMACSecret([]byte("test-secret-key-for-complete"))

	patches := gomonkey.ApplyFunc(logic.VerifyInviteToken, func(token string, secrets [][]byte) (*logic.InviteTokenPayload, error) {
		return &logic.InviteTokenPayload{
			InviteID: "inv_test1", Email: "test@example.com", Role: "user",
			ExpireAt: time.Now().Add(24 * time.Hour).Unix(),
		}, nil
	})
	defer patches.Reset()

	patches = patches.ApplyMethodFunc(invRepo, "FindByInviteID", func(ctx context.Context, inviteID string) (*model.Invite, error) {
		return &model.Invite{InviteID: "inv_test1", Status: model.InviteStatusPending, Role: "user", CreatedBy: "admin-1"}, nil
	})

	patches = patches.ApplyMethodFunc(repo, "FindByUsername", func(ctx context.Context, username string) (*model.User, error) {
		return nil, nil
	})

	patches.ApplyFunc(middleware.HashPassword, func(pw string) (string, error) { return "$2a$hashed", nil })

	patches = patches.ApplyMethodFunc(repo, "Create", func(ctx context.Context, user *model.User) error {
		return nil
	})

	patches = patches.ApplyMethodFunc(invRepo, "MarkAccepted", func(ctx context.Context, inviteID, userID string) error {
		return nil
	})

	patches = patches.ApplyMethodFunc(jwt, "GenerateToken", func(userID, username, role string) (string, error) {
		return "", errors.New("token generation failed")
	})

	_, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "valid-token", Username: "newuser", Password: "Pass123!", DisplayName: "New User",
	})
	if err == nil {
		t.Fatal("expected error from GenerateToken, got nil")
	}
}

// ===== Login: GenerateToken error =====

func TestLogin_GenerateTokenError(t *testing.T) {
	repo := &mongo.UserRepository{}
	user := &model.User{
		ID: primitive.NewObjectID(), Username: "testuser",
		PasswordHash: "$2a$10$dummy", Role: model.RoleUser, Status: model.StatusEnabled,
	}

	patches := gomonkey.ApplyMethodReturn(repo, "FindByUsername", user, nil)
	defer patches.Reset()
	patches.ApplyFunc(middleware.CheckPassword, func(hash, pw string) error { return nil })

	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	patches = patches.ApplyMethodReturn(jwt, "GenerateToken", "", errors.New("generate failed"))

	svc := NewService(repo, jwt)
	_, err := svc.Login(context.Background(), &LoginRequest{Username: "testuser", Password: "pass"})
	if err == nil {
		t.Error("should error for token generation failure")
	}
}

// ===== Register: Create user error =====

func TestRegister_CreateError(t *testing.T) {
	repo := &mongo.UserRepository{}
	patches := gomonkey.ApplyMethodReturn(repo, "FindByUsername", (*model.User)(nil), nil)
	defer patches.Reset()
	patches.ApplyFunc(middleware.HashPassword, func(pw string) (string, error) { return "$2a$hashed", nil })
	patches.ApplyMethodFunc(repo, "Create", func(ctx context.Context, user *model.User) error {
		return errors.New("create user failed")
	})

	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(repo, jwt)

	_, err := svc.Register(context.Background(), &RegisterRequest{Username: "newuser", Password: "Pass123!"})
	if err == nil {
		t.Error("should error for create user failure")
	}
}

// ===== CreateInvite: repo.Create error =====

func TestCreateInvite_CreateError(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	invRepo := &mongo.InviteRepository{}
	svc.SetInviteRepo(invRepo)
	svc.SetHMACSecret([]byte("test-secret-key-16chars"))

	patches := gomonkey.ApplyMethodFunc(invRepo, "Create", func(ctx context.Context, invite *model.Invite) error {
		return errors.New("create invite failed")
	})
	defer patches.Reset()

	_, err := svc.CreateInvite(context.Background(), "admin-1", &CreateInviteRequest{
		Email: "test@example.com", Role: "user", ExpireHours: 24,
	})
	if err == nil {
		t.Error("should error for create invite failure")
	}
}

// ===== ListInvites: repo.List error =====

func TestListInvites_ListError(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	invRepo := &mongo.InviteRepository{}
	svc.SetInviteRepo(invRepo)

	patches := gomonkey.ApplyMethodFunc(invRepo, "List", func(ctx context.Context, createdBy string, skip, limit int64) ([]model.Invite, int64, error) {
		return nil, int64(0), errors.New("list invites failed")
	})
	defer patches.Reset()

	_, err := svc.ListInvites(context.Background(), "", 1, 10)
	if err == nil {
		t.Error("should error for list failure")
	}
}

// ===== CompleteRegistration: no repo =====

func TestCompleteRegistration_NoRepo(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	svc.SetHMACSecret([]byte("test-secret-key"))

	_, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "any-token", Username: "test", Password: "Pass123!", DisplayName: "Test",
	})
	if err == nil {
		t.Error("should error without repo")
	}
}

// ===== CompleteRegistration: no HMAC secret =====

func TestCompleteRegistration_NoHMACSecret(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)
	invRepo := &mongo.InviteRepository{}
	svc.SetInviteRepo(invRepo)

	_, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "any-token", Username: "test", Password: "Pass123!", DisplayName: "Test",
	})
	if err == nil {
		t.Error("should error without HMAC secret")
	}
}
