package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
)

// TestRegisterAllRoutes_AllHandlersWired verifies that RegisterAllRoutes does
// not panic when every handler is non-nil. This exercises every register*
// helper (auth, protected, feature, workspace, admin KB, artifact, knowledge,
// audit, apireview, notification, task, dashboard) and the IMBind branch.
// Handlers are constructed with nil services because route registration only
// stores the handler reference — it does not invoke service methods.
func TestRegisterAllRoutes_AllHandlersWired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	jwt := middleware.NewJWTManager("secret", 3600)

	deps := &RouteDeps{
		JWTManager:  jwt,
		AuditLogger: nil,
		HermesURL:   "",
		AppName:     "data-agent",
		Auth:         NewAuthHandler(nil),
		User:         NewUserHandler(nil),
		Role:         NewRoleHandler(nil),
		ModelConfig:  NewModelConfigHandler(nil),
		SysConfig:    NewConfigHandler(nil, nil, nil),
		Memory:       NewMemoryHandler(nil, "data-agent"),
		Chat:         NewChatHandler(nil),
		Enhance:      NewEnhanceHandler(nil),
		Agent:        NewAgentHandler(nil, nil, nil),
		Session:      NewSessionHandler(nil),
		Artifact:     NewArtifactHandler(nil, nil),
		Knowledge:    NewKnowledgeHandler(nil),
		Audit:        NewAuditHandler(nil),
		APIReview:    NewAPIReviewHandler(nil),
		Notification: NewNotificationHandler(nil),
		Task:         NewTaskHandler(nil),
		Dashboard:    NewDashboardHandler(nil, nil, nil),
		IMBind:       NewIMBindHandler(nil),
	}
	RegisterAllRoutes(router, deps)

	// Health route should always be registered.
	routes := router.Routes()
	healthFound := false
	for _, r := range routes {
		if r.Path == "/health" {
			healthFound = true
			break
		}
	}
	if !healthFound {
		t.Error("/health route should be registered")
	}

	// A representative route from each register* helper should be present.
	wantPaths := []string{
		"/api/v1/auth/login",
		"/api/v1/auth/register",
		"/api/v1/auth/refresh",
		"/api/v1/auth/profile",
		"/api/v1/users",
		"/api/v1/roles",
		"/api/v1/permissions",
		"/api/v1/models",
		"/api/v1/sysconfig/:namespace",
		"/api/v1/change-password",
		"/api/v1/memory/search",
		"/api/v1/chat",
		"/api/v1/chat/enhance",
		"/api/v1/agent/tasks",
		"/api/v1/agent/skills",
		"/api/v1/sessions",
		"/api/v1/sessions/deleted",
		"/api/v1/artifacts/upload",
		"/api/v1/workspace/:session_id/files",
		"/api/v1/knowledge/docs",
		"/api/v1/knowledge/search",
		"/api/v1/admin/knowledge/docs",
		"/api/v1/admin/audit/logs",
		"/api/v1/admin/api-reviews",
		"/api/v1/notifications",
		"/api/v1/notifications/broadcast",
		"/api/v1/tasks",
		"/api/v1/admin/tasks",
		"/api/v1/admin/dashboard",
		"/api/v1/im/bind",
	}
	routeSet := make(map[string]bool, len(routes))
	for _, r := range routes {
		routeSet[r.Path] = true
	}
	missing := []string{}
	for _, p := range wantPaths {
		if !routeSet[p] {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		t.Errorf("missing expected routes: %v", missing)
	}
}

// TestRegisterAllRoutes_IMWebhook verifies the IM webhook branch is taken when
// IMWebhook is non-nil.
func TestRegisterAllRoutes_IMWebhook(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	jwt := middleware.NewJWTManager("secret", 3600)
	deps := &RouteDeps{
		JWTManager: jwt,
		IMWebhook:  func(http.ResponseWriter, *http.Request) {},
		AppName:    "data-agent",
	}
	RegisterAllRoutes(router, deps)
	routes := router.Routes()
	found := false
	for _, r := range routes {
		if r.Path == "/api/v1/im/feishu/webhook" {
			found = true
			break
		}
	}
	if !found {
		t.Error("feishu webhook route should be registered")
	}
}

// TestRegisterAuthRoutes_NilHandler verifies the DBUnavailable fallback branch
// for auth routes when the auth handler is nil.
func TestRegisterAuthRoutes_NilHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	authGroup := r.Group("/api/v1/auth")
	registerAuthRoutes(authGroup, nil)

	// POST /api/v1/auth/login → 503 (DBUnavailable)
	req := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for nil auth handler, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "database not available") {
		t.Errorf("expected DBUnavailable message, got %s", w.Body.String())
	}

	// POST /api/v1/auth/register → 503 (DBUnavailable)
	req = httptest.NewRequest("POST", "/api/v1/auth/register", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for nil auth handler, got %d: %s", w.Code, w.Body.String())
	}
}

// TestRegisterAuthProtected_NilHandler verifies the DBUnavailable fallback
// branch for auth-protected routes when the auth handler is nil.
func TestRegisterAuthProtected_NilHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	registerAuthProtected(api, nil)

	// POST /api/v1/auth/refresh → 503 (DBUnavailable)
	req := httptest.NewRequest("POST", "/api/v1/auth/refresh", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for nil auth handler, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "database not available") {
		t.Errorf("expected DBUnavailable message, got %s", w.Body.String())
	}

	// GET /api/v1/auth/profile → 503 (DBUnavailable)
	req = httptest.NewRequest("GET", "/api/v1/auth/profile", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for nil auth handler, got %d: %s", w.Code, w.Body.String())
	}
}

// TestRegisterAdminRoutes_NilHandler verifies the early-return branch when the
// auth handler is nil.
func TestRegisterAdminRoutes_NilHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	admin := r.Group("/api/v1/admin")
	registerAdminRoutes(admin, nil)

	// No routes should be registered under /api/v1/admin.
	routes := r.Routes()
	for _, rt := range routes {
		if strings.HasPrefix(rt.Path, "/api/v1/admin/invites") {
			t.Errorf("expected no invite routes for nil auth handler, got %s", rt.Path)
		}
	}
}
