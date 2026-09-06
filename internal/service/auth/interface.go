package auth

import "context"

//go:generate mockery --name AuthService --output ./mocks --outpkg mocks

// AuthService defines the authentication service contract.
type AuthService interface {
	Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error)
	RefreshToken(ctx context.Context, userID, username, role string) (*LoginResponse, error)

	// ChangePassword updates the current user's own password (SPEC-083).
	// The target user is derived from userID (JWT claim), never from the request body.
	ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error

	// Invite methods
	IsInviteEnabled() bool
	CreateInvite(ctx context.Context, createdBy string, req *CreateInviteRequest) (*CreateInviteResponse, error)
	ListInvites(ctx context.Context, createdBy string, page, pageSize int64) (*ListInvitesResponse, error)
	RevokeInvite(ctx context.Context, inviteID, actorUserID string, actorIsSystemAdmin bool) error
	VerifyInviteToken(ctx context.Context, token string) (*VerifyInviteResponse, error)
	CompleteRegistration(ctx context.Context, req *CompleteRegistrationRequest) (*CompleteRegistrationResponse, error)

	// HMAC
	UpdateHMACSecret(ctx context.Context, newSecret string) error
}

var _ AuthService = (*Service)(nil)
