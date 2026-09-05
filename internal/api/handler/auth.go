package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	authsvc "github.com/luoxiaojun1992/data-agent/internal/service/auth"
	"github.com/luoxiaojun1992/data-agent/internal/domain/consts"
)

// AuthHandler handles authentication-related HTTP endpoints.
type AuthHandler struct {
	authService authsvc.AuthService
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(authService authsvc.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// Login handles user login.
// POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req authsvc.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidReq + ": " + err.Error()})
		return
	}

	resp, err := h.authService.Login(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// RefreshToken handles token refresh.
// POST /api/v1/auth/refresh
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")
	role, _ := c.Get("role")

	resp, err := h.authService.RefreshToken(
		c.Request.Context(),
		userID.(string),
		username.(string),
		role.(string),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// ChangePassword updates the current user's own password (SPEC-083).
// The target user is taken solely from the JWT claim (c.Get("user_id")),
// never from a request-body field — structurally preventing any user from
// changing another user's password.
// POST /api/v1/auth/change-password
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	userID, _ := c.Get("user_id")
	userIDStr, _ := userID.(string)

	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "旧密码和新密码不能为空"})
		return
	}

	err := h.authService.ChangePassword(c.Request.Context(), userIDStr, req.OldPassword, req.NewPassword)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"message": "密码修改成功"})
	case errors.Is(err, authsvc.ErrPasswordTooWeak):
		c.JSON(http.StatusBadRequest, gin.H{"error": "密码至少 8 位，需包含大小写字母和数字"})
	case errors.Is(err, authsvc.ErrUserNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
	case errors.Is(err, authsvc.ErrWrongOldPassword):
		c.JSON(http.StatusBadRequest, gin.H{"error": "旧密码不正确"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// GetProfile returns the current user's profile.
// GET /api/v1/auth/profile
func (h *AuthHandler) GetProfile(c *gin.Context) {
	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")
	role, _ := c.Get("role")

	c.JSON(http.StatusOK, gin.H{
		"user_id":  userID,
		"username": username,
		"role":     role,
	})
}

// ── Invite Endpoints ──

// CreateInvite generates a new invite link.
// POST /api/v1/admin/invites
func (h *AuthHandler) CreateInvite(c *gin.Context) {
	userID, _ := c.Get("user_id")
	userRole, _ := c.Get("role")

	var req authsvc.CreateInviteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidReq + ": " + err.Error()})
		return
	}

	// Role-based invite restriction: no one can invite system_admin.
	if req.Role == "system_admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot invite system_admin"})
		return
	}
	// Admin can only invite regular users.
	if userRole == "admin" && req.Role != "user" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin can only invite regular users"})
		return
	}

	resp, err := h.authService.CreateInvite(c.Request.Context(), userID.(string), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, resp)
}

// ListInvites returns all invites.
// GET /api/v1/admin/invites
func (h *AuthHandler) ListInvites(c *gin.Context) {
	userID, _ := c.Get("user_id")
	userRole, _ := c.Get("role")

	page := int64(1)
	size := int64(20)
	if p := c.Query("page"); p != "" {
		if v, err := parseInt64(p); err == nil && v > 0 {
			page = v
		}
	}
	if s := c.Query("size"); s != "" {
		if v, err := parseInt64(s); err == nil && v > 0 {
			size = v
		}
	}

	// system_admin sees all; admin sees only their own
	createdBy := ""
	if userRole == "admin" {
		createdBy = userID.(string)
	}

	resp, err := h.authService.ListInvites(c.Request.Context(), createdBy, page, size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// RevokeInvite revokes a pending invite.
// DELETE /api/v1/admin/invites/:id
func (h *AuthHandler) RevokeInvite(c *gin.Context) {
	inviteID := c.Param("id")
	if err := h.authService.RevokeInvite(c.Request.Context(), inviteID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "invite revoked"})
}

// VerifyInvite validates an invite token for the registration page.
// GET /api/v1/auth/register?token=xxx
func (h *AuthHandler) VerifyInvite(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing invite token"})
		return
	}

	resp, err := h.authService.VerifyInviteToken(c.Request.Context(), token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !resp.Valid {
		c.JSON(http.StatusGone, gin.H{"error": "Invalid, expired, or used invite token"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// CompleteRegistration completes user registration with an invite token.
// POST /api/v1/auth/complete-registration
func (h *AuthHandler) CompleteRegistration(c *gin.Context) {
	var req authsvc.CompleteRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidReq + ": " + err.Error()})
		return
	}

	resp, err := h.authService.CompleteRegistration(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, resp)
}

// UpdateHMACSecret rotates the invite signing key.
// PUT /api/v1/admin/invites/hmac-secret (system_admin only)
func (h *AuthHandler) UpdateHMACSecret(c *gin.Context) {
	var req authsvc.UpdateHMACSecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidReq + ": " + err.Error()})
		return
	}

	if err := h.authService.UpdateHMACSecret(c.Request.Context(), req.NewSecret); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "hmac secret updated"})
}

// parseInt64 parses a string to int64 safely.
func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
