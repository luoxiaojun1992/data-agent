package handler

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
)

// TestRegisterAllRoutes_NilHandlers is a smoke test ensuring route registration
// does not panic when most handlers are nil (database-unavailable startup path).
// It exercises every register* helper and the Hermes/memory/health branches.
func TestRegisterAllRoutes_NilHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	jwt := middleware.NewJWTManager("secret", 3600)
	// All handlers nil; RegisterAllRoutes must skip nil branches gracefully.
	deps := &RouteDeps{
		JWTManager:  jwt,
		AuditLogger: nil,
		HermesURL:   "", // no hermes proxy
		AppName:     "data-agent",
	}
	RegisterAllRoutes(router, deps)
	// Verify health route was registered.
	routes := router.Routes()
	found := false
	for _, r := range routes {
		if r.Path == "/health" {
			found = true
			break
		}
	}
	if !found {
		t.Error("/health route should be registered")
	}
}

// TestRegisterAllRoutes_WithHermes verifies the Hermes proxy is registered when
// a URL is provided.
func TestRegisterAllRoutes_WithHermes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	jwt := middleware.NewJWTManager("secret", 3600)
	deps := &RouteDeps{
		JWTManager: jwt,
		HermesURL:  "http://localhost:9999",
		AppName:    "data-agent",
	}
	RegisterAllRoutes(router, deps)
	routes := router.Routes()
	found := false
	for _, r := range routes {
		if r.Path == "/api/v1/hermes/*path" {
			found = true
			break
		}
	}
	if !found {
		t.Error("hermes proxy route should be registered")
	}
}
