package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

// ── toString Tests ──

func TestToString(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"int", 42, ""},
		{"bool", true, ""},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toString(tt.input)
			if got != tt.want {
				t.Errorf("toString(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── truncateString Tests ──

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"shorter than max", "hello", 10, "hello"},
		{"exactly max", "hello", 5, "hello"},
		{"longer than max", "hello world", 5, "hello..."},
		{"empty string", "", 5, ""},
		{"zero max", "abc", 0, "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

// ── AuditLogger Tests ──

func TestNewAuditLogger(t *testing.T) {
	coll := &mongo.Collection{}
	logger := NewAuditLogger(coll)
	if logger == nil {
		t.Fatal("NewAuditLogger returned nil")
	}
}

func TestAuditMiddleware_GetSkipsLogging(t *testing.T) {
	gin.SetMode(gin.TestMode)
	coll := &mongo.Collection{}
	logger := NewAuditLogger(coll)

	patches := gomonkey.ApplyMethodReturn(coll, "InsertOne", &mongo.InsertOneResult{}, nil)
	defer patches.Reset()

	router := gin.New()
	router.Use(logger.AuditMiddleware())
	router.GET("/api/users", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET should return 200, got %d", w.Code)
	}
}

func TestAuditMiddleware_HeadSkipsLogging(t *testing.T) {
	gin.SetMode(gin.TestMode)
	coll := &mongo.Collection{}
	logger := NewAuditLogger(coll)

	patches := gomonkey.ApplyMethodReturn(coll, "InsertOne", &mongo.InsertOneResult{}, nil)
	defer patches.Reset()

	router := gin.New()
	router.Use(logger.AuditMiddleware())
	router.HEAD("/api/users", func(c *gin.Context) {
		c.Status(200)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, "/api/users", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("HEAD should return 200, got %d", w.Code)
	}
}

func TestAuditMiddleware_OptionsSkipsLogging(t *testing.T) {
	gin.SetMode(gin.TestMode)
	coll := &mongo.Collection{}
	logger := NewAuditLogger(coll)

	patches := gomonkey.ApplyMethodReturn(coll, "InsertOne", &mongo.InsertOneResult{}, nil)
	defer patches.Reset()

	router := gin.New()
	router.Use(logger.AuditMiddleware())
	router.OPTIONS("/api/users", func(c *gin.Context) {
		c.Status(204)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/users", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS should return 204, got %d", w.Code)
	}
}

func TestAuditMiddleware_PostLogsAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	coll := &mongo.Collection{}
	logger := NewAuditLogger(coll)

	patches := gomonkey.ApplyMethodReturn(coll, "InsertOne", &mongo.InsertOneResult{}, nil)

	router := gin.New()
	router.Use(logger.AuditMiddleware())
	router.POST("/api/users", func(c *gin.Context) {
		c.JSON(201, gin.H{"created": true})
	})

	w := httptest.NewRecorder()
	body := strings.NewReader(`{"name":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/users", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "test-agent")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		// Don't forget to reset
		patches.Reset()
		t.Fatalf("POST should return 201, got %d", w.Code)
	}

	// Wait for async goroutine to complete before resetting patches
	time.Sleep(200 * time.Millisecond)
	patches.Reset()
}

func TestAuditMiddleware_PutLogsAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	coll := &mongo.Collection{}
	logger := NewAuditLogger(coll)

	patches := gomonkey.ApplyMethodReturn(coll, "InsertOne", &mongo.InsertOneResult{}, nil)

	router := gin.New()
	router.Use(logger.AuditMiddleware())
	router.PUT("/api/users/:id", func(c *gin.Context) {
		c.JSON(200, gin.H{"updated": true})
	})

	w := httptest.NewRecorder()
	body := strings.NewReader(`{"name":"updated"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/users/123", body)
	req.Header.Set("User-Agent", "test-agent")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		patches.Reset()
		t.Fatalf("PUT should return 200, got %d", w.Code)
	}

	time.Sleep(200 * time.Millisecond)
	patches.Reset()
}

func TestAuditMiddleware_DeleteLogsAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	coll := &mongo.Collection{}
	logger := NewAuditLogger(coll)

	patches := gomonkey.ApplyMethodReturn(coll, "InsertOne", &mongo.InsertOneResult{}, nil)

	router := gin.New()
	router.Use(logger.AuditMiddleware())
	router.DELETE("/api/users/:id", func(c *gin.Context) {
		c.JSON(200, gin.H{"deleted": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/users/123", nil)
	req.Header.Set("User-Agent", "test-agent")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		patches.Reset()
		t.Fatalf("DELETE should return 200, got %d", w.Code)
	}

	time.Sleep(200 * time.Millisecond)
	patches.Reset()
}

func TestAuditMiddleware_WithUserContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	coll := &mongo.Collection{}
	logger := NewAuditLogger(coll)

	patches := gomonkey.ApplyMethodReturn(coll, "InsertOne", &mongo.InsertOneResult{}, nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "user-123")
		c.Set("username", "testuser")
		c.Next()
	})
	router.Use(logger.AuditMiddleware())
	router.POST("/api/data", func(c *gin.Context) {
		c.JSON(201, gin.H{"created": true})
	})

	w := httptest.NewRecorder()
	body := strings.NewReader(`{"data":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/data", body)
	req.Header.Set("User-Agent", "test-agent")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		patches.Reset()
		t.Fatalf("POST with user context should return 201, got %d", w.Code)
	}

	time.Sleep(200 * time.Millisecond)
	patches.Reset()
}

func TestAuditMiddleware_InsertOneError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	coll := &mongo.Collection{}
	logger := NewAuditLogger(coll)

	patches := gomonkey.ApplyMethodReturn(coll, "InsertOne", (*mongo.InsertOneResult)(nil), errAudit())

	router := gin.New()
	router.Use(logger.AuditMiddleware())
	router.POST("/api/data", func(c *gin.Context) {
		c.JSON(201, gin.H{"created": true})
	})

	w := httptest.NewRecorder()
	body := strings.NewReader(`{"name":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/data", body)
	req.Header.Set("User-Agent", "test-agent")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		patches.Reset()
		t.Fatalf("POST should return 201 even with audit error, got %d", w.Code)
	}

	time.Sleep(200 * time.Millisecond)
	patches.Reset()
}

func TestAuditMiddleware_NilBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	coll := &mongo.Collection{}
	logger := NewAuditLogger(coll)

	patches := gomonkey.ApplyMethodReturn(coll, "InsertOne", &mongo.InsertOneResult{}, nil)

	router := gin.New()
	router.Use(logger.AuditMiddleware())
	router.POST("/api/data", func(c *gin.Context) {
		c.JSON(201, gin.H{"created": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/data", nil)
	req.Header.Set("User-Agent", "test-agent")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		patches.Reset()
		t.Fatalf("POST with nil body should return 201, got %d", w.Code)
	}

	time.Sleep(200 * time.Millisecond)
	patches.Reset()
}

func TestAuditMiddleware_BodyIsRestored(t *testing.T) {
	gin.SetMode(gin.TestMode)
	coll := &mongo.Collection{}
	logger := NewAuditLogger(coll)

	patches := gomonkey.ApplyMethodReturn(coll, "InsertOne", &mongo.InsertOneResult{}, nil)

	router := gin.New()
	router.Use(logger.AuditMiddleware())
	router.POST("/api/data", func(c *gin.Context) {
		c.JSON(201, gin.H{"created": true})
	})

	w := httptest.NewRecorder()
	body := strings.NewReader(`{"test":"body-restored"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/data", body)
	req.Header.Set("User-Agent", "test-agent")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		patches.Reset()
		t.Fatalf("POST should return 201, got %d", w.Code)
	}

	time.Sleep(200 * time.Millisecond)
	patches.Reset()
}

func errAudit() error {
	return &simpleAuditErr{}
}

type simpleAuditErr struct{}

func (e *simpleAuditErr) Error() string {
	return "audit db error"
}
