package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/gin-gonic/gin"
	authsvc "github.com/luoxiaojun1992/data-agent/internal/service/auth"
	mockauth "github.com/luoxiaojun1992/data-agent/internal/service/auth/mocks"
)

// ── Helpers ──

func newGinContext(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

// ── Login Tests ──

func TestLogin_Success(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("Login", mock.Anything, mock.Anything).Return(&authsvc.LoginResponse{
		UserID: "user123", Username: "testuser", Role: "user",
		AccessToken: "token-abc", TokenType: "Bearer", ExpiresIn: 3600,
	}, nil)

	body := `{"username": "testuser", "password": "password123"}`
	c, w := newGinContext("POST", "/auth/login", body)
	h.Login(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp authsvc.LoginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Username != "testuser" {
		t.Errorf("username: got %s", resp.Username)
	}
	if resp.AccessToken != "token-abc" {
		t.Errorf("token: got %s", resp.AccessToken)
	}
}

func TestLogin_ServiceError(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("Login", mock.Anything, mock.Anything).Return((*authsvc.LoginResponse)(nil), fmt.Errorf("invalid credentials"))

	body := `{"username": "testuser", "password": "wrongpass"}`
	c, w := newGinContext("POST", "/auth/login", body)
	h.Login(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for service error, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLogin_InvalidJSON(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	c, w := newGinContext("POST", "/auth/login", "not-json")
	h.Login(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestLogin_InvalidRequest(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	// Missing required fields
	body := `{}`
	c, w := newGinContext("POST", "/auth/login", body)
	h.Login(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestLogin_EmptyBody(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	c, w := newGinContext("POST", "/auth/login", "")
	h.Login(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestLogin_MissingPassword(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	body := `{"username": "testuser"}`
	c, w := newGinContext("POST", "/auth/login", body)
	h.Login(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestLogin_MissingUsername(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	body := `{"password": "test123456"}`
	c, w := newGinContext("POST", "/auth/login", body)
	h.Login(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestLogin_AuthFailure(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("Login", mock.Anything, mock.Anything).Return((*authsvc.LoginResponse)(nil), fmt.Errorf("invalid credentials"))

	body := `{"username": "testuser", "password": "wrongpassword"}`
	c, w := newGinContext("POST", "/auth/login", body)
	h.Login(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ── Register Tests ──

func TestRegister_InviteEnabled(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("IsInviteEnabled").Return(true)

	body := `{"username": "newuser", "password": "Pass123!"}`
	c, w := newGinContext("POST", "/auth/register", body)
	h.Register(c)

	if w.Code != http.StatusGone {
		t.Errorf("expected 410, got %d", w.Code)
	}
}

func TestRegister_InviteEnabledWithInvalidJSON(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("IsInviteEnabled").Return(true)

	c, w := newGinContext("POST", "/auth/register", "not-valid-json")
	h.Register(c)

	if w.Code != http.StatusGone {
		t.Errorf("expected 410 for invite enabled regardless of JSON, got %d", w.Code)
	}
}

func TestRegister_Success(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("IsInviteEnabled").Return(false)
	svc.On("Register", mock.Anything, mock.Anything).Return(&authsvc.RegisterResponse{
		UserID:   "user456",
		Username: "newuser",
		Role:     "user",
		Message:  "Registration successful",
	}, nil)

	body := `{"username": "newuser", "password": "Pass123!", "role": "user"}`
	c, w := newGinContext("POST", "/auth/register", body)
	h.Register(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp authsvc.RegisterResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Username != "newuser" {
		t.Errorf("username: got %s", resp.Username)
	}
}

func TestRegister_InvalidJSON(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("IsInviteEnabled").Return(false)

	c, w := newGinContext("POST", "/auth/register", "not-json")
	h.Register(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRegister_Conflict(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("IsInviteEnabled").Return(false)
	svc.On("Register", mock.Anything, mock.Anything).Return((*authsvc.RegisterResponse)(nil), fmt.Errorf("username already exists"))

	body := `{"username": "existing", "password": "Pass123!"}`
	c, w := newGinContext("POST", "/auth/register", body)
	h.Register(c)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestRegister_WithAdminRole(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("IsInviteEnabled").Return(false)
	svc.On("Register", mock.Anything, mock.Anything).Return(&authsvc.RegisterResponse{
		UserID:   "user789",
		Username: "adminuser",
		Role:     "admin",
		Message:  "Registration successful",
	}, nil)

	body := `{"username": "adminuser", "password": "Pass123!", "role": "admin"}`
	c, w := newGinContext("POST", "/auth/register", body)
	h.Register(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp authsvc.RegisterResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Role != "admin" {
		t.Errorf("role: got %s, want admin", resp.Role)
	}
}

func TestRegister_ServiceError(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("IsInviteEnabled").Return(false)
	svc.On("Register", mock.Anything, mock.Anything).Return((*authsvc.RegisterResponse)(nil), fmt.Errorf("internal error"))

	body := `{"username": "newuser", "password": "Pass123!"}`
	c, w := newGinContext("POST", "/auth/register", body)
	h.Register(c)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

// ── RefreshToken Tests ──

func TestRefreshToken_Success(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("RefreshToken", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&authsvc.LoginResponse{
		UserID:      "user123",
		Username:    "testuser",
		Role:        "user",
		AccessToken: "refreshed-token",
		TokenType:   "Bearer",
		ExpiresIn:   3600,
	}, nil)

	body := `{}`
	c, w := newGinContext("POST", "/auth/refresh", body)
	c.Set("user_id", "user123")
	c.Set("username", "testuser")
	c.Set("role", "user")
	h.RefreshToken(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp authsvc.LoginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.AccessToken != "refreshed-token" {
		t.Errorf("token: got %s", resp.AccessToken)
	}
}

func TestRefreshToken_Error(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("RefreshToken", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return((*authsvc.LoginResponse)(nil), fmt.Errorf("token generation failed"))

	body := `{}`
	c, w := newGinContext("POST", "/auth/refresh", body)
	c.Set("user_id", "user123")
	c.Set("username", "testuser")
	c.Set("role", "user")
	h.RefreshToken(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── GetProfile Tests ──

func TestGetProfile_Success(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	c, w := newGinContext("GET", "/auth/profile", "")
	c.Set("user_id", "user123")
	c.Set("username", "testuser")
	c.Set("role", "admin")
	h.GetProfile(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["user_id"] != "user123" {
		t.Errorf("user_id: got %v", resp["user_id"])
	}
	if resp["username"] != "testuser" {
		t.Errorf("username: got %v", resp["username"])
	}
	if resp["role"] != "admin" {
		t.Errorf("role: got %v", resp["role"])
	}
}

func TestGetProfile_NoValues(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	c, w := newGinContext("GET", "/auth/profile", "")
	h.GetProfile(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ── CompleteRegistration Tests ──

func TestCompleteRegistration_Success(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("CompleteRegistration", mock.Anything, mock.Anything).Return(&authsvc.CompleteRegistrationResponse{
		UserID:      "user789",
		Username:    "inviteduser",
		Role:        "user",
		AccessToken: "jwt-token",
		TokenType:   "Bearer",
		ExpiresIn:   3600,
	}, nil)

	body := `{"token": "invite-token-abc", "username": "inviteduser", "password": "Pass123!", "display_name": "Invited User"}`
	c, w := newGinContext("POST", "/auth/complete-registration", body)
	h.CompleteRegistration(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp authsvc.CompleteRegistrationResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Username != "inviteduser" {
		t.Errorf("username: got %s", resp.Username)
	}
}

func TestCompleteRegistration_InvalidJSON(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	c, w := newGinContext("POST", "/auth/complete-registration", "bad")
	h.CompleteRegistration(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCompleteRegistration_Conflict(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("CompleteRegistration", mock.Anything, mock.Anything).Return((*authsvc.CompleteRegistrationResponse)(nil), fmt.Errorf("username already exists"))

	body := `{"token": "invite-token", "username": "existing", "password": "Pass123!", "display_name": "Existing"}`
	c, w := newGinContext("POST", "/auth/complete-registration", body)
	h.CompleteRegistration(c)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

// ── CreateInvite Tests ──

func TestCreateInvite_Success(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("CreateInvite", mock.Anything, mock.Anything, mock.Anything).Return(&authsvc.CreateInviteResponse{
		InviteID:  "inv_abc123",
		InviteURL: "https://example.com/register?token=xyz",
	}, nil)

	body := `{"email": "test@example.com", "role": "user", "expire_hours": 24}`
	c, w := newGinContext("POST", "/admin/invites", body)
	c.Set("user_id", "admin-1")
	c.Set("role", "system_admin")
	h.CreateInvite(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp authsvc.CreateInviteResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.InviteID != "inv_abc123" {
		t.Errorf("invite_id: got %s", resp.InviteID)
	}
}

func TestCreateInvite_InvalidJSON(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	c, w := newGinContext("POST", "/admin/invites", "bad")
	c.Set("user_id", "admin-1")
	c.Set("role", "system_admin")
	h.CreateInvite(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateInvite_SystemAdminCreatesAdminInvite(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("CreateInvite", mock.Anything, mock.Anything, mock.Anything).Return(&authsvc.CreateInviteResponse{
		InviteID:  "inv_admin001",
		InviteURL: "https://example.com/register?token=admin-token",
	}, nil)

	body := `{"email": "newadmin@example.com", "role": "admin", "expire_hours": 48}`
	c, w := newGinContext("POST", "/admin/invites", body)
	c.Set("user_id", "sys-admin-1")
	c.Set("role", "system_admin")
	h.CreateInvite(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp authsvc.CreateInviteResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.InviteID != "inv_admin001" {
		t.Errorf("invite_id: got %s", resp.InviteID)
	}
}

func TestCreateInvite_AdminCantInviteAdmin(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	body := `{"email": "admin@example.com", "role": "admin"}`
	c, w := newGinContext("POST", "/admin/invites", body)
	c.Set("user_id", "admin-2")
	c.Set("role", "admin")
	h.CreateInvite(c)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestCreateInvite_ServiceError(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("CreateInvite", mock.Anything, mock.Anything, mock.Anything).Return((*authsvc.CreateInviteResponse)(nil), fmt.Errorf("invite disabled"))

	body := `{"email": "test@example.com", "role": "user"}`
	c, w := newGinContext("POST", "/admin/invites", body)
	c.Set("user_id", "admin-1")
	c.Set("role", "system_admin")
	h.CreateInvite(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── ListInvites Tests ──

func TestListInvites_Success(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("ListInvites", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&authsvc.ListInvitesResponse{
		Invites: []authsvc.ListInviteResponse{},
		Total:   0,
		Page:    1,
		Size:    20,
	}, nil)

	c, w := newGinContext("GET", "/admin/invites", "")
	c.Set("user_id", "admin-1")
	c.Set("role", "system_admin")
	h.ListInvites(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp authsvc.ListInvitesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("total: got %d", resp.Total)
	}
}

func TestListInvites_WithPagination(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("ListInvites", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&authsvc.ListInvitesResponse{
		Invites: []authsvc.ListInviteResponse{},
		Total:   5,
		Page:    2,
		Size:    5,
	}, nil)

	c, w := newGinContext("GET", "/admin/invites?page=2&size=5", "")
	c.Set("user_id", "admin-1")
	c.Set("role", "system_admin")
	h.ListInvites(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp authsvc.ListInvitesResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Page != 2 {
		t.Errorf("page: got %d", resp.Page)
	}
}

func TestListInvites_AdminRole(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("ListInvites", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&authsvc.ListInvitesResponse{
		Invites: []authsvc.ListInviteResponse{},
		Total:   0,
		Page:    1,
		Size:    20,
	}, nil)

	c, w := newGinContext("GET", "/admin/invites", "")
	c.Set("user_id", "admin-2")
	c.Set("role", "admin")
	h.ListInvites(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestListInvites_ServiceError(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("ListInvites", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return((*authsvc.ListInvitesResponse)(nil), fmt.Errorf("db error"))

	c, w := newGinContext("GET", "/admin/invites", "")
	c.Set("user_id", "admin-1")
	c.Set("role", "system_admin")
	h.ListInvites(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestListInvites_InvalidPageParam(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	// Default page=1 size=20 should be used when page param is invalid
	svc.On("ListInvites", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&authsvc.ListInvitesResponse{
		Invites: []authsvc.ListInviteResponse{},
		Total:   0,
		Page:    1,
		Size:    20,
	}, nil)

	c, w := newGinContext("GET", "/admin/invites?page=abc&size=-1", "")
	c.Set("user_id", "admin-1")
	c.Set("role", "system_admin")
	h.ListInvites(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ── RevokeInvite Tests ──

func TestRevokeInvite_Success(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("RevokeInvite", mock.Anything, mock.Anything).Return(nil)

	c, w := newGinContext("DELETE", "/admin/invites/inv-1", "")
	c.Params = gin.Params{{Key: "id", Value: "inv-1"}}
	h.RevokeInvite(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRevokeInvite_Error(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("RevokeInvite", mock.Anything, mock.Anything).Return(fmt.Errorf("not found"))

	c, w := newGinContext("DELETE", "/admin/invites/inv-999", "")
	c.Params = gin.Params{{Key: "id", Value: "inv-999"}}
	h.RevokeInvite(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── VerifyInvite Tests ──

func TestVerifyInvite_MissingToken(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	c, w := newGinContext("GET", "/auth/register", "")
	h.VerifyInvite(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestVerifyInvite_Valid(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("VerifyInviteToken", mock.Anything, mock.Anything).Return(&authsvc.VerifyInviteResponse{
		Email: "test@example.com",
		Role:  "user",
		Valid: true,
	}, nil)

	c, w := newGinContext("GET", "/auth/register?token=valid-token", "")
	h.VerifyInvite(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp authsvc.VerifyInviteResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.Valid {
		t.Error("expected valid")
	}
}

func TestVerifyInvite_Invalid(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("VerifyInviteToken", mock.Anything, mock.Anything).Return(&authsvc.VerifyInviteResponse{
		Valid: false,
	}, nil)

	c, w := newGinContext("GET", "/auth/register?token=bad-token", "")
	h.VerifyInvite(c)

	if w.Code != http.StatusGone {
		t.Errorf("expected 410, got %d", w.Code)
	}
}

func TestVerifyInvite_ServiceError(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("VerifyInviteToken", mock.Anything, mock.Anything).Return((*authsvc.VerifyInviteResponse)(nil), fmt.Errorf("db error"))

	c, w := newGinContext("GET", "/auth/register?token=any", "")
	h.VerifyInvite(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── UpdateHMACSecret Tests ──

func TestUpdateHMACSecret_Success(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("UpdateHMACSecret", mock.Anything, mock.Anything).Return(nil)

	body := `{"new_secret": "new-secret-with-16-chars"}`
	c, w := newGinContext("PUT", "/admin/invites/hmac-secret", body)
	h.UpdateHMACSecret(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateHMACSecret_InvalidJSON(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	c, w := newGinContext("PUT", "/admin/invites/hmac-secret", "bad")
	h.UpdateHMACSecret(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestUpdateHMACSecret_ServiceError(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("UpdateHMACSecret", mock.Anything, mock.Anything).Return(fmt.Errorf("secret too short"))

	body := `{"new_secret": "too-short-secret-that-fails"}`
	c, w := newGinContext("PUT", "/admin/invites/hmac-secret", body)
	h.UpdateHMACSecret(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── NewAuthHandler Tests ──

func TestNewAuthHandler(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)
	if h == nil {
		t.Fatal("NewAuthHandler returned nil")
	}
	if h.authService != svc {
		t.Error("authService not set correctly")
	}
}

// ── parseInt64 Tests ──

func TestParseInt64_Valid(t *testing.T) {
	v, err := parseInt64("42")
	if err != nil {
		t.Fatalf("parseInt64: %v", err)
	}
	if v != 42 {
		t.Errorf("got %d", v)
	}
}

func TestParseInt64_Invalid(t *testing.T) {
	_, err := parseInt64("not-a-number")
	if err == nil {
		t.Error("should error for invalid input")
	}
}

func TestParseInt64_Negative(t *testing.T) {
	v, err := parseInt64("-1")
	if err != nil {
		t.Fatalf("parseInt64: %v", err)
	}
	if v != -1 {
		t.Errorf("got %d", v)
	}
}

func TestParseInt64_Zero(t *testing.T) {
	v, err := parseInt64("0")
	if err != nil {
		t.Fatalf("parseInt64: %v", err)
	}
	if v != 0 {
		t.Errorf("got %d", v)
	}
}

func TestParseInt64_LargeNumber(t *testing.T) {
	v, err := parseInt64("9223372036854775807")
	if err != nil {
		t.Fatalf("parseInt64: %v", err)
	}
	if v != 9223372036854775807 {
		t.Errorf("got %d", v)
	}
}

func TestParseInt64_Overflow(t *testing.T) {
	_, err := parseInt64("99999999999999999999")
	if err == nil {
		t.Error("should error on overflow")
	}
}

// ── GetProfile Edge Cases ──

func TestGetProfile_PartialValues(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	c, w := newGinContext("GET", "/auth/profile", "")
	c.Set("user_id", "user456")
	// Username and role not set (should be nil)
	h.GetProfile(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["user_id"] != "user456" {
		t.Errorf("user_id: got %v", resp["user_id"])
	}
}

// ── CreateInvite Edge Cases ──

func TestCreateInvite_DefaultRole(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("CreateInvite", mock.Anything, mock.Anything, mock.Anything).Return(&authsvc.CreateInviteResponse{
		InviteID:  "inv_user001",
		InviteURL: "https://example.com/register?token=user-token",
	}, nil)

	// No role specified in request - service defaults to "user"
	body := `{"email": "newuser@example.com", "expire_hours": 24}`
	c, w := newGinContext("POST", "/admin/invites", body)
	c.Set("user_id", "sys-admin-1")
	c.Set("role", "system_admin")
	h.CreateInvite(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateInvite_SystemAdminInvitesUser(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("CreateInvite", mock.Anything, mock.Anything, mock.Anything).Return(&authsvc.CreateInviteResponse{
		InviteID:  "inv_user002",
		InviteURL: "https://example.com/register?token=user2-token",
	}, nil)

	body := `{"email": "user2@example.com", "role": "user"}`
	c, w := newGinContext("POST", "/admin/invites", body)
	c.Set("user_id", "sys-admin-1")
	c.Set("role", "system_admin")
	h.CreateInvite(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

// ── VerifyInvite Edge Cases ──

func TestVerifyInvite_EmptyTokenValue(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	c, w := newGinContext("GET", "/auth/register?token=", "")
	h.VerifyInvite(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty token value, got %d", w.Code)
	}
}

func TestVerifyInvite_NoQueryParams(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	c, w := newGinContext("GET", "/auth/register", "")
	h.VerifyInvite(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── UpdateHMACSecret Edge Cases ──

func TestUpdateHMACSecret_EmptyBody(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	c, w := newGinContext("PUT", "/admin/invites/hmac-secret", "")
	h.UpdateHMACSecret(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestUpdateHMACSecret_ShortSecret(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	// "short" is less than min=16 chars - binding validation rejects before service call
	body := `{"new_secret": "short"}`
	c, w := newGinContext("PUT", "/admin/invites/hmac-secret", body)
	h.UpdateHMACSecret(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 from binding validation, got %d", w.Code)
	}
}

// ── ListInvites Edge Cases ──

func TestListInvites_SystemAdminSeesAll(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("ListInvites", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&authsvc.ListInvitesResponse{
		Invites: []authsvc.ListInviteResponse{
			{InviteID: "inv-1", Role: "user", Status: "pending"},
			{InviteID: "inv-2", Role: "admin", Status: "accepted"},
		},
		Total: 2,
		Page:  1,
		Size:  20,
	}, nil)

	c, w := newGinContext("GET", "/admin/invites", "")
	c.Set("user_id", "sys-admin-1")
	c.Set("role", "system_admin")
	h.ListInvites(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp authsvc.ListInvitesResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 2 {
		t.Errorf("total: got %d, want 2", resp.Total)
	}
}

func TestListInvites_AdminSeesOwn(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("ListInvites", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&authsvc.ListInvitesResponse{
		Invites: []authsvc.ListInviteResponse{
			{InviteID: "inv-3", Role: "user", Status: "pending"},
		},
		Total: 1,
		Page:  1,
		Size:  20,
	}, nil)

	c, w := newGinContext("GET", "/admin/invites", "")
	c.Set("user_id", "admin-3")
	c.Set("role", "admin")
	h.ListInvites(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestListInvites_LargePageSize(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	svc.On("ListInvites", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&authsvc.ListInvitesResponse{
		Invites: []authsvc.ListInviteResponse{},
		Total:   0,
		Page:    1,
		Size:    100,
	}, nil)

	c, w := newGinContext("GET", "/admin/invites?page=1&size=100", "")
	c.Set("user_id", "sys-admin-1")
	c.Set("role", "system_admin")
	h.ListInvites(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ── CompleteRegistration Edge Cases ──

func TestCompleteRegistration_MissingToken(t *testing.T) {
	svc := mockauth.NewAuthService(t)
	h := NewAuthHandler(svc)

	body := `{"username": "newuser", "password": "Pass123!", "display_name": "New User"}`
	c, w := newGinContext("POST", "/auth/complete-registration", body)
	h.CompleteRegistration(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing token, got %d", w.Code)
	}
}
