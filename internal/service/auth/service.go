package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/logic"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// PasswordHasher abstracts password operations for testability.
type PasswordHasher interface {
	Check(hash, password string) error
	Hash(password string) (string, error)
}

// TokenManager abstracts JWT operations for testability.
type TokenManager interface {
	GenerateToken(userID, username, role string) (string, error)
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

	token, err := s.jwtManager.GenerateToken(user.ID.Hex(), user.Username, string(user.Role))
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	expiresIn := int64(s.jwtManager.GetExpiration().Seconds())
	return &LoginResponse{
		UserID:       user.ID.Hex(),
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
		PasswordChanged: false,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	return &RegisterResponse{
		UserID:   user.ID.Hex(),
		Username: user.Username,
		Role:     string(user.Role),
		Message:  "Registration successful. You can now log in.",
	}, nil
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
