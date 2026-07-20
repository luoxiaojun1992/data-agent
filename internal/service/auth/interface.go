package auth

import "context"

//go:generate mockery --name AuthService --output ./mocks --outpkg mocks

// AuthService defines the authentication service contract.
type AuthService interface {
	Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error)
	Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error)
	RefreshToken(ctx context.Context, userID, username, role string) (*LoginResponse, error)

	// Invite methods
	IsInviteEnabled() bool
	CreateInvite(ctx context.Context, createdBy string, req *CreateInviteRequest) (*CreateInviteResponse, error)
	ListInvites(ctx context.Context, createdBy string, page, pageSize int64) (*ListInvitesResponse, error)
	RevokeInvite(ctx context.Context, inviteID string) error
	VerifyInviteToken(ctx context.Context, token string) (*VerifyInviteResponse, error)
	CompleteRegistration(ctx context.Context, req *CompleteRegistrationRequest) (*CompleteRegistrationResponse, error)

	// HMAC
	UpdateHMACSecret(ctx context.Context, newSecret string) error
}

var _ AuthService = (*Service)(nil)
