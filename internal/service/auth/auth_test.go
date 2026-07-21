package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/logic"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

// --- test helpers ---

type mockPasswordHasher struct {
	checkFn func(hash, pw string) error
	hashFn  func(pw string) (string, error)
}

func (m mockPasswordHasher) Check(hash, pw string) error { return m.checkFn(hash, pw) }
func (m mockPasswordHasher) Hash(pw string) (string, error) { return m.hashFn(pw) }

type mockTokenManager struct {
	genFn func(userID, username, role string) (string, error)
	expFn func() time.Duration
}

func (m mockTokenManager) GenerateToken(userID, username, role string) (string, error) {
	return m.genFn(userID, username, role)
}
func (m mockTokenManager) GetExpiration() time.Duration {
	if m.expFn != nil {
		return m.expFn()
	}
	return 1 * time.Hour
}

func newTokenManagerOK() mockTokenManager {
	return mockTokenManager{
		genFn: func(_, _, _ string) (string, error) { return "valid-token", nil },
	}
}

func newTokenManagerErr() mockTokenManager {
	return mockTokenManager{
		genFn: func(_, _, _ string) (string, error) { return "", errors.New("token generation failed") },
	}
}

func newPwdOK() mockPasswordHasher {
	return mockPasswordHasher{
		checkFn: func(_, _ string) error { return nil },
		hashFn:  func(_ string) (string, error) { return "$2a$hashed", nil },
	}
}

func newPwdErr() mockPasswordHasher {
	return mockPasswordHasher{
		checkFn: func(_, _ string) error { return context.DeadlineExceeded },
		hashFn:  func(_ string) (string, error) { return "", errors.New("hash failed") },
	}
}

func validInviteVerifier() InviteTokenVerifier {
	return func(token string, secrets [][]byte) (*logic.InviteTokenPayload, error) {
		return &logic.InviteTokenPayload{
			InviteID: "inv_test1",
			Email:    "test@example.com",
			Role:     "user",
			ExpireAt: time.Now().Add(24 * time.Hour).Unix(),
		}, nil
	}
}

func expiredInviteVerifier() InviteTokenVerifier {
	return func(token string, secrets [][]byte) (*logic.InviteTokenPayload, error) {
		return &logic.InviteTokenPayload{
			InviteID: "inv_expired",
			Email:    "test@example.com",
			Role:     "user",
			ExpireAt: time.Now().Add(-1 * time.Hour).Unix(),
		}, nil
	}
}

func usedInviteVerifier() InviteTokenVerifier {
	return func(token string, secrets [][]byte) (*logic.InviteTokenPayload, error) {
		return &logic.InviteTokenPayload{
			InviteID: "inv_used",
			Email:    "test@example.com",
			Role:     "user",
			ExpireAt: time.Now().Add(24 * time.Hour).Unix(),
		}, nil
	}
}

func errorInviteVerifier() InviteTokenVerifier {
	return func(token string, secrets [][]byte) (*logic.InviteTokenPayload, error) {
		return nil, context.DeadlineExceeded
	}
}

func newAuthSvc(userRepo repository.UserRepository, invRepo repository.InviteRepository, pwd mockPasswordHasher, tokMgr mockTokenManager, verifier InviteTokenVerifier, hmac []byte) *Service {
	s := &Service{
		userRepo:       userRepo,
		inviteRepo:     invRepo,
		jwtManager:     tokMgr,
		hmacSecret:     hmac,
		pwd:            pwd,
		inviteVerifier: verifier,
	}
	return s
}

// --- tests ---

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

// --- Login ---

func TestLogin_Success(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	user := &model.User{
		ID:           "507f1f77bcf86cd799439011",
		Username:     "testuser",
		PasswordHash: "$2a$10$dummy",
		Role:         model.RoleUser,
		Status:       model.StatusEnabled,
	}
	repo.On("FindByUsername", mock.Anything, "testuser").Return(user, nil)

	svc := newAuthSvc(repo, nil, newPwdOK(), newTokenManagerOK(), nil, nil)
	resp, err := svc.Login(context.Background(), &LoginRequest{Username: "testuser", Password: "pass"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if resp.AccessToken != "valid-token" {
		t.Errorf("token: got %s", resp.AccessToken)
	}
}

func TestLogin_NotFound(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	repo.On("FindByUsername", mock.Anything, "nobody").Return((*model.User)(nil), nil)
	svc := newAuthSvc(repo, nil, newPwdOK(), newTokenManagerOK(), nil, nil)
	_, err := svc.Login(context.Background(), &LoginRequest{Username: "nobody", Password: "pass"})
	if err == nil {
		t.Error("should error for nonexistent user")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	user := &model.User{Username: "testuser", PasswordHash: "hash", Role: model.RoleUser}
	repo.On("FindByUsername", mock.Anything, "testuser").Return(user, nil)
	svc := newAuthSvc(repo, nil, newPwdErr(), newTokenManagerOK(), nil, nil)
	_, err := svc.Login(context.Background(), &LoginRequest{Username: "testuser", Password: "wrong"})
	if err == nil {
		t.Error("should error for wrong password")
	}
}

func TestLogin_FindUserError(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	repo.On("FindByUsername", mock.Anything, "testuser").Return((*model.User)(nil), errors.New("database connection lost"))
	svc := newAuthSvc(repo, nil, newPwdOK(), newTokenManagerOK(), nil, nil)
	_, err := svc.Login(context.Background(), &LoginRequest{Username: "testuser", Password: "pass"})
	if err == nil {
		t.Error("should error for database error")
	}
}

func TestLogin_GenerateTokenError(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	user := &model.User{
		ID: "507f1f77bcf86cd799439012", Username: "testuser",
		PasswordHash: "$2a$10$dummy", Role: model.RoleUser, Status: model.StatusEnabled,
	}
	repo.On("FindByUsername", mock.Anything, "testuser").Return(user, nil)
	svc := newAuthSvc(repo, nil, newPwdOK(), newTokenManagerErr(), nil, nil)
	_, err := svc.Login(context.Background(), &LoginRequest{Username: "testuser", Password: "pass"})
	if err == nil {
		t.Error("should error for token generation failure")
	}
}

// --- Register ---

func TestRegister_Success(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	repo.On("FindByUsername", mock.Anything, "newuser").Return((*model.User)(nil), nil)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)
	svc := newAuthSvc(repo, nil, newPwdOK(), newTokenManagerOK(), nil, nil)
	resp, err := svc.Register(context.Background(), &RegisterRequest{Username: "newuser", Password: "Pass123!", Role: model.RoleUser})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if resp.Username != "newuser" {
		t.Errorf("username: got %s", resp.Username)
	}
}

func TestRegister_Duplicate(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	existing := &model.User{Username: "exists"}
	repo.On("FindByUsername", mock.Anything, "exists").Return(existing, nil)
	svc := newAuthSvc(repo, nil, newPwdOK(), newTokenManagerOK(), nil, nil)
	_, err := svc.Register(context.Background(), &RegisterRequest{Username: "exists", Password: "Pass123!"})
	if err == nil {
		t.Error("should error for duplicate")
	}
}

func TestRegister_FindUserError(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	repo.On("FindByUsername", mock.Anything, "newuser").Return((*model.User)(nil), errors.New("database error"))
	svc := newAuthSvc(repo, nil, newPwdOK(), newTokenManagerOK(), nil, nil)
	_, err := svc.Register(context.Background(), &RegisterRequest{Username: "newuser", Password: "Pass123!"})
	if err == nil {
		t.Error("should error for database error")
	}
}

func TestRegister_DefaultRole(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	repo.On("FindByUsername", mock.Anything, "newuser2").Return((*model.User)(nil), nil)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)
	svc := newAuthSvc(repo, nil, newPwdOK(), newTokenManagerOK(), nil, nil)
	resp, err := svc.Register(context.Background(), &RegisterRequest{Username: "newuser2", Password: "Pass123!"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if resp.Role != "user" {
		t.Errorf("role should default to 'user', got %q", resp.Role)
	}
}

func TestRegister_HashPasswordError(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	repo.On("FindByUsername", mock.Anything, "newuser").Return((*model.User)(nil), nil)
	svc := newAuthSvc(repo, nil, newPwdErr(), newTokenManagerOK(), nil, nil)
	_, err := svc.Register(context.Background(), &RegisterRequest{Username: "newuser", Password: "Pass123!"})
	if err == nil {
		t.Error("should error for hash failure")
	}
}

func TestRegister_AdminRole(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	repo.On("FindByUsername", mock.Anything, "admin2").Return((*model.User)(nil), nil)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)
	svc := newAuthSvc(repo, nil, newPwdOK(), newTokenManagerOK(), nil, nil)
	resp, err := svc.Register(context.Background(), &RegisterRequest{Username: "admin2", Password: "Pass123!", Role: model.RoleAdmin})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if resp.Role != "admin" {
		t.Errorf("role should be 'admin', got %q", resp.Role)
	}
}

func TestRegister_CreateError(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	repo.On("FindByUsername", mock.Anything, "newuser").Return((*model.User)(nil), nil)
	repo.On("Create", mock.Anything, mock.Anything).Return(errors.New("create user failed"))
	svc := newAuthSvc(repo, nil, newPwdOK(), newTokenManagerOK(), nil, nil)
	_, err := svc.Register(context.Background(), &RegisterRequest{Username: "newuser", Password: "Pass123!"})
	if err == nil {
		t.Error("should error for create user failure")
	}
}

// --- RefreshToken ---

func TestRefreshToken(t *testing.T) {
	svc := newAuthSvc(nil, nil, newPwdOK(), newTokenManagerOK(), nil, nil)
	resp, err := svc.RefreshToken(context.Background(), "uid", "uname", "user")
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if resp.AccessToken != "valid-token" {
		t.Errorf("token: got %s", resp.AccessToken)
	}
}

func TestRefreshToken_GenerateError(t *testing.T) {
	svc := newAuthSvc(nil, nil, newPwdOK(), newTokenManagerErr(), nil, nil)
	_, err := svc.RefreshToken(context.Background(), "uid", "uname", "user")
	if err == nil {
		t.Error("should error for token generation failure")
	}
}

// --- Invite helpers ---

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

// --- CreateInvite ---

func TestCreateInvite(t *testing.T) {
	invRepo := mockrepo.NewInviteRepository(t)
	invRepo.On("Create", mock.Anything, mock.Anything).Return(nil)
	svc := newAuthSvc(nil, invRepo, newPwdOK(), newTokenManagerOK(), validInviteVerifier(), []byte("test-secret-key-at-least-16"))
	resp, err := svc.CreateInvite(context.Background(), "admin-1", &CreateInviteRequest{
		Email: "test@example.com", Role: "user", ExpireHours: 24,
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
	svc := newAuthSvc(nil, nil, newPwdOK(), newTokenManagerOK(), nil, []byte("test-secret-key"))
	_, err := svc.CreateInvite(context.Background(), "admin-1", &CreateInviteRequest{})
	if err == nil {
		t.Error("should error without repo")
	}
}

func TestCreateInvite_SystemAdminBlocked(t *testing.T) {
	invRepo := mockrepo.NewInviteRepository(t)
	svc := newAuthSvc(nil, invRepo, newPwdOK(), newTokenManagerOK(), nil, []byte("test-secret-key-16chars"))
	_, err := svc.CreateInvite(context.Background(), "admin-1", &CreateInviteRequest{Role: "system_admin"})
	if err == nil {
		t.Error("should error for system_admin role invite")
	}
}

func TestCreateInvite_NoHMACSecret(t *testing.T) {
	invRepo := mockrepo.NewInviteRepository(t)
	svc := newAuthSvc(nil, invRepo, newPwdOK(), newTokenManagerOK(), nil, nil)
	_, err := svc.CreateInvite(context.Background(), "admin-1", &CreateInviteRequest{Email: "test@example.com", Role: "user"})
	if err == nil {
		t.Error("should error without HMAC secret")
	}
}

func TestCreateInvite_DefaultExpireHours(t *testing.T) {
	invRepo := mockrepo.NewInviteRepository(t)
	invRepo.On("Create", mock.Anything, mock.Anything).Return(nil)
	svc := newAuthSvc(nil, invRepo, newPwdOK(), newTokenManagerOK(), nil, []byte("test-secret-key-16chars"))
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
	invRepo := mockrepo.NewInviteRepository(t)
	invRepo.On("Create", mock.Anything, mock.Anything).Return(nil)
	svc := newAuthSvc(nil, invRepo, newPwdOK(), newTokenManagerOK(), nil, []byte("test-secret-key-16chars"))
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

func TestCreateInvite_CreateError(t *testing.T) {
	invRepo := mockrepo.NewInviteRepository(t)
	invRepo.On("Create", mock.Anything, mock.Anything).Return(errors.New("create invite failed"))
	svc := newAuthSvc(nil, invRepo, newPwdOK(), newTokenManagerOK(), nil, []byte("test-secret-key-16chars"))
	_, err := svc.CreateInvite(context.Background(), "admin-1", &CreateInviteRequest{
		Email: "test@example.com", Role: "user", ExpireHours: 24,
	})
	if err == nil {
		t.Error("should error for create invite failure")
	}
}

// --- ListInvites ---

func TestListInvites(t *testing.T) {
	invRepo := mockrepo.NewInviteRepository(t)
	invRepo.On("List", mock.Anything, "", int64(0), int64(10)).Return([]model.Invite{}, int64(0), nil)
	svc := newAuthSvc(nil, invRepo, newPwdOK(), newTokenManagerOK(), nil, nil)
	resp, err := svc.ListInvites(context.Background(), "", 1, 10)
	if err != nil {
		t.Fatalf("ListInvites: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("Total: got %d", resp.Total)
	}
}

func TestListInvites_NoRepo(t *testing.T) {
	svc := newAuthSvc(nil, nil, newPwdOK(), newTokenManagerOK(), nil, nil)
	_, err := svc.ListInvites(context.Background(), "", 1, 10)
	if err == nil {
		t.Error("should error without repo")
	}
}

func TestListInvites_DefaultPagination(t *testing.T) {
	invRepo := mockrepo.NewInviteRepository(t)
	invRepo.On("List", mock.Anything, "", int64(0), int64(20)).Return([]model.Invite{
		{InviteID: "inv_1", Status: model.InviteStatusPending, Role: "user", CreatedBy: "admin-1"},
	}, int64(1), nil)
	svc := newAuthSvc(nil, invRepo, newPwdOK(), newTokenManagerOK(), nil, nil)
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
	invRepo := mockrepo.NewInviteRepository(t)
	now := time.Now()
	invRepo.On("List", mock.Anything, "", int64(0), int64(10)).Return([]model.Invite{
		{InviteID: "inv_1", Email: "a@test.com", Role: "user", Status: model.InviteStatusPending, CreatedBy: "admin", CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour)},
		{InviteID: "inv_2", Email: "b@test.com", Role: "admin", Status: model.InviteStatusAccepted, CreatedBy: "admin", CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour)},
	}, int64(2), nil)
	svc := newAuthSvc(nil, invRepo, newPwdOK(), newTokenManagerOK(), nil, nil)
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

func TestListInvites_ListError(t *testing.T) {
	invRepo := mockrepo.NewInviteRepository(t)
	invRepo.On("List", mock.Anything, "", int64(0), int64(10)).Return(nil, int64(0), errors.New("list invites failed"))
	svc := newAuthSvc(nil, invRepo, newPwdOK(), newTokenManagerOK(), nil, nil)
	_, err := svc.ListInvites(context.Background(), "", 1, 10)
	if err == nil {
		t.Error("should error for list failure")
	}
}

// --- RevokeInvite ---

func TestRevokeInvite(t *testing.T) {
	invRepo := mockrepo.NewInviteRepository(t)
	invRepo.On("Revoke", mock.Anything, "inv-1").Return(nil)
	svc := newAuthSvc(nil, invRepo, newPwdOK(), newTokenManagerOK(), nil, nil)
	err := svc.RevokeInvite(context.Background(), "inv-1")
	if err != nil {
		t.Fatalf("RevokeInvite: %v", err)
	}
}

func TestRevokeInvite_NoRepo(t *testing.T) {
	svc := newAuthSvc(nil, nil, newPwdOK(), newTokenManagerOK(), nil, nil)
	err := svc.RevokeInvite(context.Background(), "inv-1")
	if err == nil {
		t.Error("should error without repo")
	}
}

// --- VerifyInviteToken ---

func TestVerifyInviteToken(t *testing.T) {
	invRepo := mockrepo.NewInviteRepository(t)

	t.Run("invalid token", func(t *testing.T) {
		invRepo2 := mockrepo.NewInviteRepository(t)
		svc := newAuthSvc(nil, invRepo2, newPwdOK(), newTokenManagerOK(), errorInviteVerifier(), []byte("test-secret-key-for-verify"))
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
		svc3 := newAuthSvc(nil, mockrepo.NewInviteRepository(t), newPwdOK(), newTokenManagerOK(), validInviteVerifier(), []byte("test-secret-key-for-verify"))
		svc3.inviteRepo.(*mockrepo.InviteRepository).On("FindByInviteID", mock.Anything, mock.Anything).Return(&model.Invite{
			InviteID: "inv_test1", Status: model.InviteStatusPending, Role: "user", Email: "test@example.com",
		}, nil)
		resp, err := svc3.VerifyInviteToken(context.Background(), "valid-token")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !resp.Valid { t.Error("token should be valid") }
		if resp.Email != "test@example.com" { t.Errorf("Email = %q", resp.Email) }
		if resp.Role != "user" { t.Errorf("Role = %q", resp.Role) }
	})

	t.Run("expired token", func(t *testing.T) {
		svc4 := newAuthSvc(nil, invRepo, newPwdOK(), newTokenManagerOK(), expiredInviteVerifier(), []byte("test-secret-key-for-verify"))
		resp, err := svc4.VerifyInviteToken(context.Background(), "expired-token")
		if err != nil { t.Fatalf("unexpected error: %v", err) }
		if resp.Valid { t.Error("expired token should not be valid") }
	})

	t.Run("already used invite", func(t *testing.T) {
		svc5 := newAuthSvc(nil, mockrepo.NewInviteRepository(t), newPwdOK(), newTokenManagerOK(), usedInviteVerifier(), []byte("test-secret-key-for-verify"))
		svc5.inviteRepo.(*mockrepo.InviteRepository).On("FindByInviteID", mock.Anything, mock.Anything).Return(&model.Invite{
			InviteID: "inv_used", Status: model.InviteStatusAccepted,
		}, nil)
		resp, err := svc5.VerifyInviteToken(context.Background(), "used-token")
		if err != nil { t.Fatalf("unexpected error: %v", err) }
		if resp.Valid { t.Error("already used invite should not be valid") }
	})

	t.Run("invite not found in DB", func(t *testing.T) {
		svc6 := newAuthSvc(nil, mockrepo.NewInviteRepository(t), newPwdOK(), newTokenManagerOK(), validInviteVerifier(), []byte("test-secret-key-for-verify"))
		svc6.inviteRepo.(*mockrepo.InviteRepository).On("FindByInviteID", mock.Anything, mock.Anything).Return((*model.Invite)(nil), nil)
		resp, err := svc6.VerifyInviteToken(context.Background(), "missing-token")
		if err != nil { t.Fatalf("unexpected error: %v", err) }
		if resp.Valid { t.Error("invite not found should not be valid") }
	})

	t.Run("no hmac secret", func(t *testing.T) {
		svc7 := &Service{}
		svc7.SetInviteRepo(invRepo)
		_, err := svc7.VerifyInviteToken(context.Background(), "any-token")
		if err == nil { t.Error("should error without HMAC secret") }
	})
}

// --- CompleteRegistration ---

func TestCompleteRegistration(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	repo.On("FindByUsername", mock.Anything, "newuser").Return((*model.User)(nil), nil)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)

	invRepo := mockrepo.NewInviteRepository(t)
	invRepo.On("FindByInviteID", mock.Anything, mock.Anything).Return(&model.Invite{
		InviteID: "inv_test1", Status: model.InviteStatusPending, Role: "user", CreatedBy: "admin-1",
	}, nil)
	invRepo.On("MarkAccepted", mock.Anything, "inv_test1", mock.Anything).Return(nil)

	svc := newAuthSvc(repo, invRepo, newPwdOK(), newTokenManagerOK(), validInviteVerifier(), []byte("test-secret-key-for-complete-reg"))
	resp, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "bW9jay10b2tlbg==.mocktoken", Username: "newuser", Password: "Pass123!", DisplayName: "New User",
	})
	if err != nil {
		t.Fatalf("CompleteRegistration: %v", err)
	}
	if resp.Username != "newuser" {
		t.Errorf("username: got %s", resp.Username)
	}
}

func TestCompleteRegistration_UsernameConflict(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	repo.On("FindByUsername", mock.Anything, "existinguser").Return(&model.User{Username: "existinguser"}, nil)
	invRepo := mockrepo.NewInviteRepository(t)
	invRepo.On("FindByInviteID", mock.Anything, mock.Anything).Return(&model.Invite{
		InviteID: "inv_test1", Status: model.InviteStatusPending, Role: "user", CreatedBy: "admin-1",
	}, nil)
	svc := newAuthSvc(repo, invRepo, newPwdOK(), newTokenManagerOK(), validInviteVerifier(), []byte("test-secret-key-for-complete"))
	_, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "valid-token", Username: "existinguser", Password: "Pass123!", DisplayName: "New User",
	})
	if err == nil { t.Fatal("expected username conflict error, got nil") }
}

func TestCompleteRegistration_ExpiredToken(t *testing.T) {
	invRepo := mockrepo.NewInviteRepository(t)
	svc := newAuthSvc(nil, invRepo, newPwdOK(), newTokenManagerOK(), expiredInviteVerifier(), []byte("test-secret-key-for-complete"))
	_, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "expired-token", Username: "newuser", Password: "Pass123!", DisplayName: "New User",
	})
	if err == nil { t.Fatal("expected expired token error, got nil") }
}

func TestCompleteRegistration_InviteNotPending(t *testing.T) {
	invRepo := mockrepo.NewInviteRepository(t)
	invRepo.On("FindByInviteID", mock.Anything, mock.Anything).Return(&model.Invite{
		InviteID: "inv_used", Status: model.InviteStatusRevoked, CreatedBy: "admin-1",
	}, nil)
	svc := newAuthSvc(nil, invRepo, newPwdOK(), newTokenManagerOK(), usedInviteVerifier(), []byte("test-secret-key-for-complete"))
	_, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "used-token", Username: "newuser", Password: "Pass123!", DisplayName: "New User",
	})
	if err == nil { t.Fatal("expected error for revoked invite, got nil") }
}

func TestCompleteRegistration_FindUserError(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	repo.On("FindByUsername", mock.Anything, "newuser").Return(nil, errors.New("db error"))
	invRepo := mockrepo.NewInviteRepository(t)
	invRepo.On("FindByInviteID", mock.Anything, mock.Anything).Return(&model.Invite{
		InviteID: "inv_test1", Status: model.InviteStatusPending, Role: "user", CreatedBy: "admin-1",
	}, nil)
	svc := newAuthSvc(repo, invRepo, newPwdOK(), newTokenManagerOK(), validInviteVerifier(), []byte("test-secret-key-for-complete"))
	_, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "valid-token", Username: "newuser", Password: "Pass123!", DisplayName: "New User",
	})
	if err == nil { t.Fatal("expected error from FindByUsername, got nil") }
}

func TestCompleteRegistration_CreateUserError(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	repo.On("FindByUsername", mock.Anything, "newuser").Return((*model.User)(nil), nil)
	repo.On("Create", mock.Anything, mock.Anything).Return(errors.New("create user failed"))
	invRepo := mockrepo.NewInviteRepository(t)
	invRepo.On("FindByInviteID", mock.Anything, mock.Anything).Return(&model.Invite{
		InviteID: "inv_test1", Status: model.InviteStatusPending, Role: "user", CreatedBy: "admin-1",
	}, nil)
	svc := newAuthSvc(repo, invRepo, newPwdOK(), newTokenManagerOK(), validInviteVerifier(), []byte("test-secret-key-for-complete"))
	_, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "valid-token", Username: "newuser", Password: "Pass123!", DisplayName: "New User",
	})
	if err == nil { t.Fatal("expected error from Create user, got nil") }
}

func TestCompleteRegistration_GenerateTokenError(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	repo.On("FindByUsername", mock.Anything, "newuser").Return((*model.User)(nil), nil)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)
	invRepo := mockrepo.NewInviteRepository(t)
	invRepo.On("FindByInviteID", mock.Anything, mock.Anything).Return(&model.Invite{
		InviteID: "inv_test1", Status: model.InviteStatusPending, Role: "user", CreatedBy: "admin-1",
	}, nil)
	invRepo.On("MarkAccepted", mock.Anything, "inv_test1", mock.Anything).Return(nil)
	svc := newAuthSvc(repo, invRepo, newPwdOK(), newTokenManagerErr(), validInviteVerifier(), []byte("test-secret-key-for-complete"))
	_, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "valid-token", Username: "newuser", Password: "Pass123!", DisplayName: "New User",
	})
	if err == nil { t.Fatal("expected error from GenerateToken, got nil") }
}

func TestCompleteRegistration_NoRepo(t *testing.T) {
	svc := newAuthSvc(nil, nil, newPwdOK(), newTokenManagerOK(), nil, []byte("test-secret-key"))
	_, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "any-token", Username: "test", Password: "Pass123!", DisplayName: "Test",
	})
	if err == nil { t.Error("should error without repo") }
}

func TestCompleteRegistration_NoHMACSecret(t *testing.T) {
	invRepo := mockrepo.NewInviteRepository(t)
	svc := newAuthSvc(nil, invRepo, newPwdOK(), newTokenManagerOK(), nil, nil)
	_, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "any-token", Username: "test", Password: "Pass123!", DisplayName: "Test",
	})
	if err == nil { t.Error("should error without HMAC secret") }
}

func TestCompleteRegistration_InviteNil(t *testing.T) {
	invRepo := mockrepo.NewInviteRepository(t)
	invRepo.On("FindByInviteID", mock.Anything, mock.Anything).Return((*model.Invite)(nil), nil)
	svc := newAuthSvc(nil, invRepo, newPwdOK(), newTokenManagerOK(), validInviteVerifier(), []byte("test-secret-key-for-complete"))
	_, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "valid-token", Username: "newuser", Password: "Pass123!", DisplayName: "New User",
	})
	if err == nil { t.Fatal("expected error for nil invite, got nil") }
}

func TestCompleteRegistration_HashPasswordError(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	repo.On("FindByUsername", mock.Anything, "newuser").Return((*model.User)(nil), nil)
	invRepo := mockrepo.NewInviteRepository(t)
	invRepo.On("FindByInviteID", mock.Anything, mock.Anything).Return(&model.Invite{
		InviteID: "inv_test1", Status: model.InviteStatusPending, Role: "user", CreatedBy: "admin-1",
	}, nil)
	svc := newAuthSvc(repo, invRepo, newPwdErr(), newTokenManagerOK(), validInviteVerifier(), []byte("test-secret-key-for-complete"))
	_, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "valid-token", Username: "newuser", Password: "Pass123!", DisplayName: "New User",
	})
	if err == nil { t.Fatal("expected HashPassword error, got nil") }
}

func TestCompleteRegistration_FindInviteError(t *testing.T) {
	invRepo := mockrepo.NewInviteRepository(t)
	invRepo.On("FindByInviteID", mock.Anything, mock.Anything).Return((*model.Invite)(nil), errors.New("database connection lost"))
	svc := newAuthSvc(nil, invRepo, newPwdOK(), newTokenManagerOK(), validInviteVerifier(), []byte("test-secret-key-for-complete"))
	_, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "valid-token", Username: "newuser", Password: "Pass123!", DisplayName: "New User",
	})
	if err == nil { t.Fatal("expected error from FindByInviteID, got nil") }
}

func TestCompleteRegistration_VerifyTokenError(t *testing.T) {
	repo := mockrepo.NewUserRepository(t)
	invRepo := mockrepo.NewInviteRepository(t)
	svc := newAuthSvc(repo, invRepo, newPwdOK(), newTokenManagerOK(), errorInviteVerifier(), []byte("test-secret-key-for-verify-err"))
	_, err := svc.CompleteRegistration(context.Background(), &CompleteRegistrationRequest{
		Token: "any-token", Username: "u", Password: "Pass123!",
	})
	if err == nil { t.Fatal("expected error from VerifyInviteToken") }
}

// --- ComputeTokenHash ---

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
