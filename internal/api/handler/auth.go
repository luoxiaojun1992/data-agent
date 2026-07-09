package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	authsvc "github.com/luoxiaojun1992/data-agent/internal/service/auth"
)

// AuthHandler handles authentication-related HTTP endpoints.
type AuthHandler struct {
	authService *authsvc.Service
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(authService *authsvc.Service) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// Login handles user login.
// POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req authsvc.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	resp, err := h.authService.Login(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// Register handles user registration.
// POST /api/v1/auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	var req authsvc.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	resp, err := h.authService.Register(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, resp)
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
