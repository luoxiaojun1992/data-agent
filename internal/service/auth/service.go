package auth

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/logic"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// Sentinel errors for ChangePassword (SPEC-083), allowing the handler to map
// each failure to the correct HTTP status without inspecting message strings.
var (
	ErrPasswordTooWeak  = errors.New("password must be at least 8 chars with upper, lower and digit")
	ErrUserNotFound     = errors.New("user not found")
	ErrWrongOldPassword = errors.New("old password incorrect")
)

// ConfigCache is the minimal config read interface needed by auth Service.
type ConfigCache interface {
	Get(ctx context.Context, key string) (*model.SystemConfig, error)
}

// PasswordHasher abstracts password operations for testability.
type PasswordHasher interface {
	Check(hash, password string) error
	Hash(password string) (string, error)
}

// TokenManager abstracts JWT operations for testability.
type TokenManager interface {
	GenerateToken(userID, username, role string) (string, error)
	GenerateTokenWithExpiration(userID, username, role string, expiration time.Duration) (string, error)
	GetExpiration() time.Duration
}

// InviteTokenVerifier is a function that verifies an invite token.
type InviteTokenVerifier func(token string, secrets [][]byte) (*logic.InviteTokenPayload, error)

type defaultPasswordHasher struct{}

func (defaultPasswordHasher) Check(hash, password string) error {
	return middleware.CheckPassword(hash, password)
}
func (defaultPasswordHasher) Hash(password string) (string, error) {
	return middleware.HashPassword(password)
}

// Service handles authentication and authorization business logic.
type Service struct {
	userRepo       repository.UserRepository
	inviteRepo     repository.InviteRepository
	jwtManager     TokenManager
	hmacSecret     []byte
	pwd            PasswordHasher
	inviteVerifier InviteTokenVerifier
	configCache    ConfigCache // nil = use default expiration
}

// NewService creates a new auth service.
func NewService(userRepo repository.UserRepository, jwtManager *middleware.JWTManager) *Service {
	return &Service{
		userRepo:   userRepo,
		jwtManager: jwtManager,
		pwd:        defaultPasswordHasher{},
		inviteVerifier: func(token string, secrets [][]byte) (*logic.InviteTokenPayload, error) {
			return logic.VerifyInviteToken(token, secrets)
		},
	}
}

// SetSysConfigCache sets the config cache used to read runtime overrides
// such as SESSION_TIMEOUT. Pass nil to fall back to default expiration.
func (s *Service) SetSysConfigCache(c ConfigCache) {
	s.configCache = c
}

// LoginRequest represents a login request.
type LoginRequest struct {
	Username string `json:"username" binding:"required,min=2,max=50"`
	Password string `json:"password" binding:"required,min=6,max=100"`
}

// LoginResponse represents a successful login response.
type LoginResponse struct {
	UserID       string `json:"user_id"`
	Username     string `json:"username"`
	Role         string `json:"role"`
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	NeedChangePw bool   `json:"need_change_pw"`
}

// RegisterRequest represents a user registration request.
type RegisterRequest struct {
	Username string         `json:"username" binding:"required,min=2,max=50"`
	Password string         `json:"password" binding:"required,min=6,max=100"`
	Role     model.UserRole `json:"role,omitempty"`
}

// RegisterResponse represents a successful registration.
type RegisterResponse struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Message  string `json:"message"`
}

// Login authenticates a user and returns a JWT token.
func (s *Service) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	user, err := s.userRepo.FindByUsername(ctx, req.Username)
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("invalid username or password")
	}

	if err := s.pwd.Check(user.PasswordHash, req.Password); err != nil {
		return nil, fmt.Errorf("invalid username or password")
	}

	if user.Status != model.StatusEnabled {
		return nil, fmt.Errorf("account disabled")
	}

	expiration := s.jwtManager.GetExpiration() // default 24h
	if s.configCache != nil {
		if cfg, err := s.configCache.Get(ctx, "SESSION_TIMEOUT"); err == nil && cfg != nil && cfg.Value != "" {
			if h, err := strconv.ParseInt(cfg.Value, 10, 64); err == nil && h > 0 {
				expiration = time.Duration(h) * time.Hour
			}
		}
	}

	token, err := s.jwtManager.GenerateTokenWithExpiration(user.ID, user.Username, string(user.Role), expiration)
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	expiresIn := int64(expiration.Seconds())
	return &LoginResponse{
		UserID:       user.ID,
		Username:     user.Username,
		Role:         string(user.Role),
		AccessToken:  token,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		NeedChangePw: !user.PasswordChanged,
	}, nil
}

// Register creates a new user account.
func (s *Service) Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	existing, err := s.userRepo.FindByUsername(ctx, req.Username)
	if err != nil {
		return nil, fmt.Errorf("check existing user: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("username already exists")
	}

	passwordHash, err := s.pwd.Hash(req.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	role := req.Role
	if role != model.RoleAdmin && role != model.RoleUser {
		role = model.RoleUser
	}

	user := &model.User{
		Username:        req.Username,
		PasswordHash:    passwordHash,
		Role:            role,
		Status:          model.StatusEnabled,
		PasswordChanged: false,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	return &RegisterResponse{
		UserID:   user.ID,
		Username: user.Username,
		Role:     string(user.Role),
		Message:  "Registration successful. You can now log in.",
	}, nil
}

// ChangePassword updates the current user's own password (SPEC-083).
// The target user is always userID (derived from the JWT claim), never from any
// request-body field — this is what makes "each user can only change their own
// password" structurally enforced rather than a convention.
//
// Order: complexity check → resolve user → verify old password → hash new → persist.
func (s *Service) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	if !validatePasswordComplexity(newPassword) {
		return ErrPasswordTooWeak
	}

	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil || user == nil {
		return ErrUserNotFound
	}

	if err := s.pwd.Check(user.PasswordHash, oldPassword); err != nil {
		return ErrWrongOldPassword
	}

	newHash, err := s.pwd.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}

	if err := s.userRepo.UpdatePassword(ctx, userID, newHash); err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	return nil
}

// validatePasswordComplexity enforces ≥8 chars with at least one upper, one
// lower and one digit. Migrated from the old ConfigHandler (SPEC-083) so the
// rule lives with the auth domain and is unit-testable without a Gin context.
func validatePasswordComplexity(pw string) bool {
	hasUpper, hasLower, hasDigit := false, false, false
	for _, c := range pw {
		switch {
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= '0' && c <= '9':
			hasDigit = true
		}
	}
	return len(pw) >= 8 && hasUpper && hasLower && hasDigit
}

// RefreshToken generates a new token for an existing authenticated user.
func (s *Service) RefreshToken(ctx context.Context, userID, username, role string) (*LoginResponse, error) {
	token, err := s.jwtManager.GenerateToken(userID, username, role)
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	expiresIn := int64(s.jwtManager.GetExpiration().Seconds())
	return &LoginResponse{
		UserID:      userID,
		Username:    username,
		Role:        role,
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   expiresIn,
	}, nil
}
