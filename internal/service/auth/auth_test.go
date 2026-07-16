package auth

import (
	"context"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
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
		PasswordHash: "$2a$10$dummyhashedpasswordxxx",
		Role:         model.RoleUser,
		Status:       model.StatusEnabled,
	}

	patches := gomonkey.ApplyMethodSeq(repo, "FindByUsername", []gomonkey.OutputCell{
		{Values: gomonkey.Params{user, nil}},
	})
	defer patches.Reset()

	patches.ApplyFunc(middleware.CheckPassword, func(hash, pw string) error {
		return nil
	})

	patches.ApplyMethodSeq(&middleware.JWTManager{}, "GenerateToken", []gomonkey.OutputCell{
		{Values: gomonkey.Params{"valid-token-xxx", nil}},
	})

	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(repo, jwt)

	resp, err := svc.Login(context.Background(), &LoginRequest{
		Username: "testuser",
		Password: "password",
	})
	if err != nil {
		t.Fatalf("Login error: %v", err)
	}
	if resp.AccessToken == "" {
		t.Error("AccessToken should not be empty")
	}
	if resp.Username != "testuser" {
		t.Errorf("Username: got %s", resp.Username)
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	repo := &mongo.UserRepository{}
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(repo, jwt)

	patches := gomonkey.ApplyMethodSeq(repo, "FindByUsername", []gomonkey.OutputCell{
		{Values: gomonkey.Params{nil, nil}},
	})
	defer patches.Reset()

	_, err := svc.Login(context.Background(), &LoginRequest{
		Username: "nobody",
		Password: "password",
	})
	if err == nil {
		t.Error("should error for nonexistent user")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	repo := &mongo.UserRepository{}
	user := &model.User{
		ID:           primitive.NewObjectID(),
		Username:     "testuser",
		PasswordHash: "hash",
		Role:         model.RoleUser,
	}

	patches := gomonkey.ApplyMethodSeq(repo, "FindByUsername", []gomonkey.OutputCell{
		{Values: gomonkey.Params{user, nil}},
	})
	defer patches.Reset()

	patches.ApplyFunc(middleware.CheckPassword, func(hash, pw string) error {
		return context.DeadlineExceeded // simulate wrong password
	})

	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(repo, jwt)

	_, err := svc.Login(context.Background(), &LoginRequest{
		Username: "testuser",
		Password: "wrong",
	})
	if err == nil {
		t.Error("should error for wrong password")
	}
}

func TestRegister_Success(t *testing.T) {
	repo := &mongo.UserRepository{}
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(repo, jwt)

	patches := gomonkey.ApplyMethodSeq(repo, "FindByUsername", []gomonkey.OutputCell{
		{Values: gomonkey.Params{nil, nil}},
	})
	defer patches.Reset()

	patches.ApplyFunc(middleware.HashPassword, func(pw string) (string, error) {
		return "$2a$hashed", nil
	})

	patches.ApplyMethodSeq(repo, "Create", []gomonkey.OutputCell{
		{Values: gomonkey.Params{nil}},
	})

	resp, err := svc.Register(context.Background(), &RegisterRequest{
		Username: "newuser",
		Password: "Pass123!",
		Role:     model.RoleUser,
	})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if resp.Username != "newuser" {
		t.Errorf("Username: got %s", resp.Username)
	}
}

func TestRegister_DuplicateUser(t *testing.T) {
	repo := &mongo.UserRepository{}
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(repo, jwt)

	existing := &model.User{Username: "exists"}
	patches := gomonkey.ApplyMethodSeq(repo, "FindByUsername", []gomonkey.OutputCell{
		{Values: gomonkey.Params{existing, nil}},
	})
	defer patches.Reset()

	_, err := svc.Register(context.Background(), &RegisterRequest{
		Username: "exists",
		Password: "Pass123!",
	})
	if err == nil {
		t.Error("should error for duplicate username")
	}
}

func TestRefreshToken(t *testing.T) {
	jwt := middleware.NewJWTManager("test", 1*time.Hour)
	svc := NewService(nil, jwt)

	patches := gomonkey.ApplyMethodSeq(&middleware.JWTManager{}, "GenerateToken", []gomonkey.OutputCell{
		{Values: gomonkey.Params{"refreshed-token", nil}},
	})
	defer patches.Reset()

	resp, err := svc.RefreshToken(context.Background(), "uid", "uname", "user")
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if resp.AccessToken != "refreshed-token" {
		t.Error("should return refreshed token")
	}
}

func TestSetInviteRepo(t *testing.T) {
	svc := &Service{}
	svc.SetInviteRepo(nil)
	if svc.inviteRepo != nil {
		t.Error("SetInviteRepo with nil should set nil")
	}
}

func TestSetHMACSecret(t *testing.T) {
	svc := &Service{}
	svc.SetHMACSecret([]byte("my-secret"))
	if string(svc.hmacSecret) != "my-secret" {
		t.Error("SetHMACSecret should store the secret")
	}
}
