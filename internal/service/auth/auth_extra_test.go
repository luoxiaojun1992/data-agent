package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

// --- NewService ---

// TestNewService_Defaults verifies NewService wires the repository, jwt manager,
// default password hasher and the default invite verifier onto the Service.
func TestNewService_Defaults(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	jwt := middleware.NewJWTManager("test-secret-16chars-long", time.Hour)

	svc := NewService(repo, jwt)

	if svc == nil {
		t.Fatal("NewService should return a non-nil Service")
	}
	if svc.userRepo != repo {
		t.Error("userRepo should be the injected repository")
	}
	if svc.jwtManager == nil {
		t.Error("jwtManager should be set")
	}
	if _, ok := svc.pwd.(defaultPasswordHasher); !ok {
		t.Errorf("pwd should be defaultPasswordHasher, got %T", svc.pwd)
	}
	if svc.inviteVerifier == nil {
		t.Error("inviteVerifier should be set by default to logic.VerifyInviteToken")
	}
	// hmacSecret and inviteRepo should be zero-valued until Set* is called.
	if len(svc.hmacSecret) != 0 {
		t.Errorf("hmacSecret should be empty by default, got %q", string(svc.hmacSecret))
	}
	if svc.inviteRepo != nil {
		t.Error("inviteRepo should be nil until SetInviteRepo is called")
	}
}

// TestNewService_RefreshTokenUsesJWTExpiration exercises NewService end-to-end
// by calling RefreshToken and verifying the JWT manager's expiration is reflected.
func TestNewService_RefreshTokenUsesJWTExpiration(t *testing.T) {
	jwt := middleware.NewJWTManager("test-secret-16chars-long", 2*time.Hour)
	svc := NewService(mockrepo.NewUserRepository(t), jwt)

	resp, err := svc.RefreshToken(context.Background(), "uid-1", "alice", "admin")
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if resp.AccessToken == "" {
		t.Error("AccessToken should not be empty")
	}
	if resp.TokenType != "Bearer" {
		t.Errorf("TokenType = %q, want Bearer", resp.TokenType)
	}
	wantExp := int64((2 * time.Hour).Seconds())
	if resp.ExpiresIn != wantExp {
		t.Errorf("ExpiresIn = %d, want %d", resp.ExpiresIn, wantExp)
	}
	if resp.UserID != "uid-1" || resp.Username != "alice" || resp.Role != "admin" {
		t.Errorf("identity fields wrong: %+v", resp)
	}
}

// TestNewService_LoginEndToEnd wires NewService with a real JWT manager and the
// default bcrypt password hasher, then exercises Login with a correct password.
func TestNewService_LoginEndToEnd(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	jwt := middleware.NewJWTManager("test-secret-16chars-long", time.Hour)
	svc := NewService(repo, jwt)

	hash, err := svc.pwd.Hash("MyPass123!")
	if err != nil {
		t.Fatalf("seed hash: %v", err)
	}
	user := &model.User{
		ID: "507f1f77bcf86cd799439099", Username: "alice",
		PasswordHash: hash, Role: model.RoleUser, Status: model.StatusEnabled,
		PasswordChanged: true,
	}
	repo.On("FindByUsername", mock.Anything, "alice").Return(user, nil)

	resp, err := svc.Login(context.Background(), &LoginRequest{Username: "alice", Password: "MyPass123!"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if resp.AccessToken == "" {
		t.Error("AccessToken should not be empty")
	}
	if resp.NeedChangePw != false {
		t.Errorf("NeedChangePw = %v, want false (PasswordChanged=true)", resp.NeedChangePw)
	}
	if resp.UserID != "507f1f77bcf86cd799439099" {
		t.Errorf("UserID = %q", resp.UserID)
	}
}

// TestNewService_LoginWrongPasswordEndToEnd verifies NewService's default hasher
// rejects an incorrect password during Login.
func TestNewService_LoginWrongPasswordEndToEnd(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	jwt := middleware.NewJWTManager("test-secret-16chars-long", time.Hour)
	svc := NewService(repo, jwt)

	hash, _ := svc.pwd.Hash("CorrectPass123!")
	user := &model.User{
		ID: "u2", Username: "bob",
		PasswordHash: hash, Role: model.RoleUser, Status: model.StatusEnabled,
	}
	repo.On("FindByUsername", mock.Anything, "bob").Return(user, nil)

	_, err := svc.Login(context.Background(), &LoginRequest{Username: "bob", Password: "WrongPass456!"})
	if err == nil {
		t.Fatal("Login should fail for wrong password")
	}
	if !strings.Contains(err.Error(), "invalid username or password") {
		t.Errorf("error should mention invalid credentials, got %v", err)
	}
}

// TestNewService_RegisterEndToEnd exercises the default hasher via Register and
// confirms the admin role is preserved end-to-end.
func TestNewService_RegisterEndToEnd(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	jwt := middleware.NewJWTManager("test-secret-16chars-long", time.Hour)
	svc := NewService(repo, jwt)

	repo.On("FindByUsername", mock.Anything, "newuser3").Return((*model.User)(nil), nil)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		// Simulate the repo assigning an ID to the user.
		u := args.Get(1).(*model.User)
		u.ID = "generated-id"
	})

	resp, err := svc.Register(context.Background(), &RegisterRequest{
		Username: "newuser3", Password: "Pass1234!", Role: model.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if resp.Role != "admin" {
		t.Errorf("Role = %q, want admin", resp.Role)
	}
	if resp.Username != "newuser3" {
		t.Errorf("Username = %q, want newuser3", resp.Username)
	}
	if resp.Message == "" {
		t.Error("Message should not be empty")
	}
}

// --- defaultPasswordHasher ---

// TestDefaultPasswordHasher_HashAndCheck verifies the default password hasher
// produces a bcrypt-style hash that Check then accepts for the right password.
func TestDefaultPasswordHasher_HashAndCheck(t *testing.T) {
	h := defaultPasswordHasher{}

	hash, err := h.Hash("SuperSecret123!")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$2") {
		t.Errorf("hash should be bcrypt-formatted, got %q", hash)
	}
	if hash == "SuperSecret123!" {
		t.Error("hash should not equal the plaintext password")
	}
	if err := h.Check(hash, "SuperSecret123!"); err != nil {
		t.Errorf("Check should accept the correct password: %v", err)
	}
}

// TestDefaultPasswordHasher_CheckWrongPassword verifies the default hasher
// rejects a password that does not match the hash.
func TestDefaultPasswordHasher_CheckWrongPassword(t *testing.T) {
	h := defaultPasswordHasher{}

	hash, err := h.Hash("CorrectPassword123!")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if err := h.Check(hash, "WrongPassword456!"); err == nil {
		t.Error("Check should fail for a wrong password")
	}
	// Sanity: the correct password still validates, proving the hash is good.
	if err := h.Check(hash, "CorrectPassword123!"); err != nil {
		t.Errorf("Check should accept the correct password: %v", err)
	}
}

// TestDefaultPasswordHasher_CheckInvalidHash verifies the default hasher
// returns an error for malformed hash inputs (covers the error branch in Check).
func TestDefaultPasswordHasher_CheckInvalidHash(t *testing.T) {
	h := defaultPasswordHasher{}

	if err := h.Check("not-a-real-hash", "any"); err == nil {
		t.Error("Check should fail for a malformed hash")
	}
	if err := h.Check("", "any"); err == nil {
		t.Error("Check should fail for an empty hash")
	}
	// Different hashes for different inputs proves Check is not a constant pass.
	if err := h.Check("$2a$10$invalid", "any"); err == nil {
		t.Error("Check should fail for a truncated bcrypt hash")
	}
}

// TestDefaultPasswordHasher_HashProducesUniqueHashes verifies that hashing the
// same password twice yields different salts (bcrypt behavior).
func TestDefaultPasswordHasher_HashProducesUniqueHashes(t *testing.T) {
	h := defaultPasswordHasher{}

	h1, err := h.Hash("SamePassword123!")
	if err != nil {
		t.Fatalf("Hash #1: %v", err)
	}
	h2, err := h.Hash("SamePassword123!")
	if err != nil {
		t.Fatalf("Hash #2: %v", err)
	}
	if h1 == h2 {
		t.Error("bcrypt should produce different hashes for the same password (salt)")
	}
	// Both hashes should still validate against the plaintext.
	if err := h.Check(h1, "SamePassword123!"); err != nil {
		t.Errorf("Check h1: %v", err)
	}
	if err := h.Check(h2, "SamePassword123!"); err != nil {
		t.Errorf("Check h2: %v", err)
	}
}

// --- NewService + invite verifier integration ---

// TestNewService_DefaultInviteVerifierErrors verifies the default invite
// verifier (logic.VerifyInviteToken) returns an error for a garbage token,
// which the service surfaces as an invalid-token response.
func TestNewService_DefaultInviteVerifierErrors(t *testing.T) {
	invRepo := mockrepo.NewInviteRepository(t)
	jwt := middleware.NewJWTManager("test-secret-16chars-long", time.Hour)
	svc := NewService(mockrepo.NewUserRepository(t), jwt)
	svc.SetInviteRepo(invRepo)
	svc.SetHMACSecret([]byte("test-secret-16chars-long"))

	resp, err := svc.VerifyInviteToken(context.Background(), "garbage-token-not-a-real-invite")
	if err != nil {
		t.Fatalf("VerifyInviteToken should not return a hard error for bad tokens: %v", err)
	}
	if resp == nil {
		t.Fatal("resp should not be nil")
	}
	if resp.Valid {
		t.Error("garbage token should not be valid")
	}
	if resp.Email != "" {
		t.Errorf("Email should be empty for invalid token, got %q", resp.Email)
	}
}

// TestNewService_DefaultInviteVerifierNoHMAC verifies VerifyInviteToken errors
// when HMAC secret is unset on a NewService-constructed service.
func TestNewService_DefaultInviteVerifierNoHMAC(t *testing.T) {
	invRepo := mockrepo.NewInviteRepository(t)
	jwt := middleware.NewJWTManager("test-secret-16chars-long", time.Hour)
	svc := NewService(mockrepo.NewUserRepository(t), jwt)
	svc.SetInviteRepo(invRepo)

	_, err := svc.VerifyInviteToken(context.Background(), "any-token")
	if err == nil {
		t.Error("VerifyInviteToken should error without HMAC secret")
	}
	if !strings.Contains(err.Error(), "hmac") {
		t.Errorf("error should mention hmac, got %v", err)
	}
}
