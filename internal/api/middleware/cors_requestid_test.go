package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// ── CORS Middleware Tests ──

func TestCORSMiddleware_Options(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORSMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS should return 204, got %d", w.Code)
	}

	// Verify CORS headers
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Allow-Origin: got %q, want *", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("Allow-Methods should not be empty")
	}
	if got := w.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Error("Allow-Headers should not be empty")
	}
}

func TestCORSMiddleware_NormalRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORSMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET should return 200, got %d", w.Code)
	}

	// CORS headers should still be set for non-OPTIONS
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Allow-Origin: got %q, want *", got)
	}
}

func TestCORSMiddleware_AllMethods(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORSMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})
	router.POST("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})
	router.PUT("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})
	router.DELETE("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	methods := []string{"GET", "POST", "PUT", "DELETE"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(method, "/test", nil)
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("%s should return 200, got %d", method, w.Code)
			}
		})
	}
}

func TestCORSMiddleware_ExposeHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORSMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Expose-Headers"); got != "Content-Length" {
		t.Errorf("Expose-Headers: got %q, want Content-Length", got)
	}
	if got := w.Header().Get("Access-Control-Max-Age"); got != "86400" {
		t.Errorf("Max-Age: got %q, want 86400", got)
	}
}

// ── RequestID Middleware Tests ──

func TestRequestIDMiddleware_ExistingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.GET("/test", func(c *gin.Context) {
		requestID, exists := c.Get("request_id")
		if !exists {
			c.JSON(500, gin.H{"error": "no request_id"})
			return
		}
		c.JSON(200, gin.H{"request_id": requestID})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-ID", "my-custom-id-123")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("X-Request-ID"); got != "my-custom-id-123" {
		t.Errorf("X-Request-ID header should be preserved: got %q, want %q", got, "my-custom-id-123")
	}
}

func TestRequestIDMiddleware_NoHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.GET("/test", func(c *gin.Context) {
		requestID, exists := c.Get("request_id")
		if !exists {
			c.JSON(500, gin.H{"error": "no request_id"})
			return
		}
		c.JSON(200, gin.H{"request_id": requestID})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Should generate a request ID
	respHeader := w.Header().Get("X-Request-ID")
	if respHeader == "" {
		t.Error("X-Request-ID header should be generated")
	}
	if respHeader == "unknown" {
		t.Error("generated request ID should not be 'unknown' in normal case")
	}
}

func TestRequestIDMiddleware_GeneratedIDFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// Generate two request IDs and verify they're different
	w1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(w1, req1)
	rID1 := w1.Header().Get("X-Request-ID")

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(w2, req2)
	rID2 := w2.Header().Get("X-Request-ID")

	if rID1 == rID2 {
		t.Error("successive request IDs should be different")
	}
	// Should be 8 hex characters (4 bytes)
	if len(rID1) != 8 {
		t.Errorf("request ID should be 8 hex chars, got %d: %q", len(rID1), rID1)
	}
}
