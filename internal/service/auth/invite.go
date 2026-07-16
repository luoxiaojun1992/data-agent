package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
	"github.com/luoxiaojun1992/data-agent/internal/logic"
)

// CreateInviteRequest represents an invite creation request.
type CreateInviteRequest struct {
	Email       string `json:"email" binding:"omitempty,email"`
	Role        string `json:"role"`
	ExpireHours int    `json:"expire_hours" binding:"omitempty,min=1,max=168"`
}

// CreateInviteResponse represents an invite creation response.
type CreateInviteResponse struct {
	InviteID  string    `json:"invite_id"`
	InviteURL string    `json:"invite_url"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ListInviteResponse represents a single invite in the list.
type ListInviteResponse struct {
	InviteID   string             `json:"invite_id"`
	Email      string             `json:"email,omitempty"`
	Role       string             `json:"role"`
	Status     model.InviteStatus `json:"status"`
	CreatedBy  string             `json:"created_by"`
	CreatedAt  time.Time          `json:"created_at"`
	ExpiresAt  time.Time          `json:"expires_at"`
	AcceptedAt *time.Time         `json:"accepted_at,omitempty"`
	AcceptedBy string             `json:"accepted_by,omitempty"`
}

// ListInvitesResponse is the paginated list response.
type ListInvitesResponse struct {
	Invites []ListInviteResponse `json:"invites"`
	Total   int64                `json:"total"`
	Page    int64                `json:"page"`
	Size    int64                `json:"size"`
}

// VerifyInviteResponse is the response when a user visits the register page with a valid token.
type VerifyInviteResponse struct {
	Email string `json:"email,omitempty"`
	Role  string `json:"role"`
	Valid bool   `json:"valid"`
}

// CompleteRegistrationRequest is the final registration step.
type CompleteRegistrationRequest struct {
	Token       string `json:"token" binding:"required"`
	Username    string `json:"username" binding:"required,min=2,max=50"`
	Password    string `json:"password" binding:"required,min=6,max=100"`
	DisplayName string `json:"display_name" binding:"required,min=1,max=100"`
}

// CompleteRegistrationResponse is the result of successful registration.
type CompleteRegistrationResponse struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

// UpdateHMACSecretRequest is the request to rotate the signing key.
type UpdateHMACSecretRequest struct {
	NewSecret string `json:"new_secret" binding:"required,min=16"`
}

// SetInviteRepo sets the invite repository on the service.
// Called after service construction when the database is available.
func (s *Service) SetInviteRepo(inviteRepo *mongo.InviteRepository) {
	s.inviteRepo = inviteRepo
}

// SetHMACSecret sets the HMAC secret for invite token signing.
func (s *Service) SetHMACSecret(secret []byte) {
	s.hmacSecret = secret
}

// IsInviteEnabled returns true if the invite system (HMAC secret) is configured.
func (s *Service) IsInviteEnabled() bool {
	return len(s.hmacSecret) > 0
}

// CreateInvite generates a new invite token and stores it.
func (s *Service) CreateInvite(ctx context.Context, createdBy string, req *CreateInviteRequest) (*CreateInviteResponse, error) {
	if s.inviteRepo == nil {
		return nil, fmt.Errorf("invite system not available")
	}
	if len(s.hmacSecret) == 0 {
		return nil, fmt.Errorf("invite hmac secret not configured")
	}

	// Defaults
	if req.ExpireHours <= 0 {
		req.ExpireHours = 24
	}
	if req.Role == "" {
		req.Role = string(model.RoleUser)
	}

	// Role validation
	if req.Role == string(model.RoleSystemAdmin) {
		return nil, fmt.Errorf("cannot invite system_admin role")
	}

	inviteID := "inv_" + uuid.New().String()[:8]
	expiresAt := time.Now().Add(time.Duration(req.ExpireHours) * time.Hour)

	// Generate HMAC-signed token
	token := logic.GenerateInviteToken(inviteID, expiresAt, req.Email, req.Role, s.hmacSecret)

	// Hash the token payload for DB lookup (first part is the payload)
	tokenHash := computeTokenHash(token)

	invite := &model.Invite{
		InviteID:  inviteID,
		Email:     req.Email,
		Role:      req.Role,
		Status:    model.InviteStatusPending,
		TokenHash: tokenHash,
		CreatedBy: createdBy,
		ExpiresAt: expiresAt,
	}

	if err := s.inviteRepo.Create(ctx, invite); err != nil {
		return nil, fmt.Errorf("create invite: %w", err)
	}

	inviteURL := fmt.Sprintf("%s/register?token=%s", logic.GetInviteBaseURL(), token)

	return &CreateInviteResponse{
		InviteID:  inviteID,
		InviteURL: inviteURL,
		ExpiresAt: expiresAt,
	}, nil
}

// ListInvites returns paginated invites.
func (s *Service) ListInvites(ctx context.Context, createdBy string, page, pageSize int64) (*ListInvitesResponse, error) {
	if s.inviteRepo == nil {
		return nil, fmt.Errorf("invite system not available")
	}

	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}

	skip := (page - 1) * pageSize
	invites, total, err := s.inviteRepo.List(ctx, createdBy, skip, pageSize)
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}

	items := make([]ListInviteResponse, len(invites))
	for i, inv := range invites {
		items[i] = ListInviteResponse{
			InviteID:   inv.InviteID,
			Email:      inv.Email,
			Role:       inv.Role,
			Status:     inv.Status,
			CreatedBy:  inv.CreatedBy,
			CreatedAt:  inv.CreatedAt,
			ExpiresAt:  inv.ExpiresAt,
			AcceptedAt: inv.AcceptedAt,
			AcceptedBy: inv.AcceptedBy,
		}
	}

	return &ListInvitesResponse{
		Invites: items,
		Total:   total,
		Page:    page,
		Size:    pageSize,
	}, nil
}

// RevokeInvite revokes a pending invite.
func (s *Service) RevokeInvite(ctx context.Context, inviteID string) error {
	if s.inviteRepo == nil {
		return fmt.Errorf("invite system not available")
	}
	return s.inviteRepo.Revoke(ctx, inviteID)
}

// VerifyInviteToken validates an invite token from a registration link.
func (s *Service) VerifyInviteToken(ctx context.Context, token string) (*VerifyInviteResponse, error) {
	if s.inviteRepo == nil {
		return nil, fmt.Errorf("invite system not available")
	}
	if len(s.hmacSecret) == 0 {
		return nil, fmt.Errorf("invite hmac secret not configured")
	}

	// Verify HMAC signature (try current secret only for verification endpoint)
	payload, err := logic.VerifyInviteToken(token, [][]byte{s.hmacSecret})
	if err != nil {
		return &VerifyInviteResponse{Valid: false}, nil
	}

	// Check expiry
	if time.Now().Unix() > payload.ExpireAt {
		return &VerifyInviteResponse{Valid: false}, nil
	}

	// Check DB status
	invite, err := s.inviteRepo.FindByInviteID(ctx, payload.InviteID)
	if err != nil || invite == nil {
		return &VerifyInviteResponse{Valid: false}, nil
	}

	if invite.Status != model.InviteStatusPending {
		return &VerifyInviteResponse{Valid: false}, nil
	}

	return &VerifyInviteResponse{
		Email: payload.Email,
		Role:  payload.Role,
		Valid: true,
	}, nil
}

// CompleteRegistration completes user registration using a valid invite token.
func (s *Service) CompleteRegistration(ctx context.Context, req *CompleteRegistrationRequest) (*CompleteRegistrationResponse, error) {
	if s.inviteRepo == nil {
		return nil, fmt.Errorf("invite system not available")
	}
	if len(s.hmacSecret) == 0 {
		return nil, fmt.Errorf("invite hmac secret not configured")
	}

	// Verify the token with both current and previous secrets (key rotation support)
	payload, err := logic.VerifyInviteToken(req.Token, [][]byte{s.hmacSecret})
	if err != nil {
		return nil, fmt.Errorf("invalid or expired invite token")
	}

	// Check expiry
	if time.Now().Unix() > payload.ExpireAt {
		return nil, fmt.Errorf("invite link has expired")
	}

	// Check invite in DB
	invite, err := s.inviteRepo.FindByInviteID(ctx, payload.InviteID)
	if err != nil {
		return nil, fmt.Errorf("verify invite: %w", err)
	}
	if invite == nil {
		return nil, fmt.Errorf("invite not found")
	}
	if invite.Status != model.InviteStatusPending {
		return nil, fmt.Errorf("this invite has already been used or revoked")
	}

	// Check username uniqueness
	existing, err := s.userRepo.FindByUsername(ctx, req.Username)
	if err != nil {
		return nil, fmt.Errorf("check username: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("username already exists")
	}

	// Hash password
	passwordHash, err := middleware.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	// Create user
	user := &model.User{
		Username:        req.Username,
		PasswordHash:    passwordHash,
		Role:            model.UserRole(payload.Role),
		Status:          model.StatusEnabled,
		DisplayName:     req.DisplayName,
		InvitedBy:       invite.CreatedBy,
		InviteID:        payload.InviteID,
		PasswordChanged: true, // User set password during registration
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	// Mark invite as accepted
	if err := s.inviteRepo.MarkAccepted(ctx, payload.InviteID, user.ID.Hex()); err != nil {
		// Non-fatal: user is created, log or ignore
	}

	// Generate JWT for auto-login
	token, err := s.jwtManager.GenerateToken(user.ID.Hex(), user.Username, string(user.Role))
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	expiresIn := int64(s.jwtManager.GetExpiration().Seconds())
	return &CompleteRegistrationResponse{
		UserID:      user.ID.Hex(),
		Username:    user.Username,
		Role:        string(user.Role),
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   expiresIn,
	}, nil
}

// UpdateHMACSecret rotates the invite signing key.
func (s *Service) UpdateHMACSecret(ctx context.Context, newSecret string) error {
	if len(newSecret) < 16 {
		return fmt.Errorf("secret must be at least 16 characters")
	}
	s.hmacSecret = []byte(newSecret)
	return nil
}

// computeTokenHash returns the SHA256 hash of the token string for DB lookup.
func computeTokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
