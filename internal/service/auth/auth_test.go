package auth

import (
	"context"
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
}
