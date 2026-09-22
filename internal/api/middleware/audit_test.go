package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
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
	repo := mockrepo.NewAuditRepository(t)
	logger := NewAuditLogger(repo)
	if logger == nil {
		t.Fatal("NewAuditLogger returned nil")
	}
}

// expectCreate sets up the audit repo mock to expect a Create call and return nil.
func expectCreate(repo *mockrepo.AuditRepository) {
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)
}

// expectCreateError sets up the audit repo mock to return an error on Create.
func expectCreateError(repo *mockrepo.AuditRepository, err error) {
	repo.On("Create", mock.Anything, mock.Anything).Return(err)
}

func TestAuditMiddleware_GetSkipsLogging(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := mockrepo.NewAuditRepository(t)
	logger := NewAuditLogger(repo)

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
	// GET must not trigger an audit write.
	time.Sleep(50 * time.Millisecond)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestAuditMiddleware_HeadSkipsLogging(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := mockrepo.NewAuditRepository(t)
	logger := NewAuditLogger(repo)

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
	time.Sleep(50 * time.Millisecond)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestAuditMiddleware_OptionsSkipsLogging(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := mockrepo.NewAuditRepository(t)
	logger := NewAuditLogger(repo)

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
	time.Sleep(50 * time.Millisecond)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestAuditMiddleware_PostLogsAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := mockrepo.NewAuditRepository(t)
	logger := NewAuditLogger(repo)
	expectCreate(repo)

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
		t.Fatalf("POST should return 201, got %d", w.Code)
	}

	// Wait for the async audit goroutine to call repo.Create.
	time.Sleep(200 * time.Millisecond)
	repo.AssertCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestAuditMiddleware_PutLogsAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := mockrepo.NewAuditRepository(t)
	logger := NewAuditLogger(repo)
	expectCreate(repo)

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
		t.Fatalf("PUT should return 200, got %d", w.Code)
	}

	time.Sleep(200 * time.Millisecond)
	repo.AssertCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestAuditMiddleware_DeleteLogsAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := mockrepo.NewAuditRepository(t)
	logger := NewAuditLogger(repo)
	expectCreate(repo)

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
		t.Fatalf("DELETE should return 200, got %d", w.Code)
	}

	time.Sleep(200 * time.Millisecond)
	repo.AssertCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestAuditMiddleware_WithUserContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := mockrepo.NewAuditRepository(t)
	logger := NewAuditLogger(repo)
	expectCreate(repo)

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
		t.Fatalf("POST with user context should return 201, got %d", w.Code)
	}

	time.Sleep(200 * time.Millisecond)
	repo.AssertCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestAuditMiddleware_InsertOneError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := mockrepo.NewAuditRepository(t)
	logger := NewAuditLogger(repo)
	expectCreateError(repo, errAudit())

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
		t.Fatalf("POST should return 201 even with audit error, got %d", w.Code)
	}

	// Audit error must not propagate to the response. The Create call still
	// happens (best-effort), so assert it was invoked.
	time.Sleep(200 * time.Millisecond)
	repo.AssertCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestAuditMiddleware_NilBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := mockrepo.NewAuditRepository(t)
	logger := NewAuditLogger(repo)
	expectCreate(repo)

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
		t.Fatalf("POST with nil body should return 201, got %d", w.Code)
	}

	time.Sleep(200 * time.Millisecond)
	repo.AssertCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestAuditMiddleware_BodyNotConsumed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := mockrepo.NewAuditRepository(t)
	logger := NewAuditLogger(repo)
	expectCreate(repo)

	var receivedBody string
	router := gin.New()
	router.Use(logger.AuditMiddleware())
	router.POST("/api/data", func(c *gin.Context) {
		// Read body inside the handler to prove the middleware did not consume
		// it (the middleware no longer reads the body at all).
		buf := make([]byte, 1024)
		n, _ := c.Request.Body.Read(buf)
		receivedBody = string(buf[:n])
		c.JSON(201, gin.H{"created": true})
	})

	w := httptest.NewRecorder()
	body := strings.NewReader(`{"test":"body-not-consumed"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/data", body)
	req.Header.Set("User-Agent", "test-agent")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("POST should return 201, got %d", w.Code)
	}
	if receivedBody != `{"test":"body-not-consumed"}` {
		t.Errorf("body must be left intact for downstream handler: got %q", receivedBody)
	}

	time.Sleep(200 * time.Millisecond)
	repo.AssertCalled(t, "Create", mock.Anything, mock.Anything)
}

// ── sanitizeQuery Tests ──

func TestSanitizeQuery(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty query", "", ""},
		{"plain params untouched", "page=2&limit=10", "page=2&limit=10"},
		{"token redacted", "token=abc123", "token=***"},
		{"access_token redacted", "access_token=secret", "access_token=***"},
		{"authorization redacted", "authorization=Bearer+xyz", "authorization=***"},
		{"password redacted", "password=p@ss", "password=***"},
		{"mixed sensitive and plain", "token=abc&page=2", "token=***&page=2"},
		{"key case-insensitive", "Token=abc", "Token=***"},
		{"non-sensitive malformed value preserved", "a=%zz", "a=%zz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeQuery(tt.in)
			if got != tt.want {
				t.Errorf("sanitizeQuery(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestAuditMiddleware_DetailsExcludesBodyAndToken asserts that neither the
// request body nor the Authorization header token nor a query-string token ends
// up in the persisted audit Details.
func TestAuditMiddleware_DetailsExcludesBodyAndToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := mockrepo.NewAuditRepository(t)
	logger := NewAuditLogger(repo)

	var captured *model.AuditLog
	repo.On("Create", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { captured = args.Get(1).(*model.AuditLog) }).
		Return(nil)

	router := gin.New()
	router.Use(logger.AuditMiddleware())
	router.POST("/api/v1/chat", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	body := strings.NewReader(`{"password":"secret123","content":"huge-file-bytes"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat?token=abc123&page=2", body)
	req.Header.Set("Authorization", "Bearer top-secret-token")
	req.Header.Set("User-Agent", "test-agent")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST should return 200, got %d", w.Code)
	}

	time.Sleep(200 * time.Millisecond)
	if captured == nil {
		t.Fatal("audit log was not captured")
	}
	if strings.Contains(captured.Details, "secret123") || strings.Contains(captured.Details, "huge-file-bytes") {
		t.Errorf("Details must not contain the request body, got %q", captured.Details)
	}
	if strings.Contains(captured.Details, "abc123") || strings.Contains(captured.Details, "top-secret-token") {
		t.Errorf("Details must not contain any token, got %q", captured.Details)
	}
	if captured.Details != "token=***&page=2" {
		t.Errorf("Details = %q, want %q", captured.Details, "token=***&page=2")
	}
}

func errAudit() error {
	return &simpleAuditErr{}
}

type simpleAuditErr struct{}

func (e *simpleAuditErr) Error() string {
	return "audit db error"
}
