// Package main is the entry point for the DataAgent server.
// SPEC-003: Infrastructure setup with full middleware stack.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/config"
	"github.com/luoxiaojun1992/data-agent/internal/domain/agent"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
	agent_svc "github.com/luoxiaojun1992/data-agent/internal/service/agent"
	artifact_svc "github.com/luoxiaojun1992/data-agent/internal/service/artifact"
	"github.com/luoxiaojun1992/data-agent/internal/api/handler"
	"github.com/luoxiaojun1992/data-agent/internal/service/chat"
	"github.com/luoxiaojun1992/data-agent/internal/service/knowledge"
	mongoinfra "github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
	"github.com/luoxiaojun1992/data-agent/internal/infra/seaweedfs"
	"github.com/luoxiaojun1992/data-agent/internal/logic/workspace"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize logger
	logger, err := initLogger(cfg)
	if err != nil {
		log.Fatalf("Failed to init logger: %v", err)
	}
	defer func() {
		if err := logger.Sync(); err != nil {
			log.Printf("logger sync error: %v", err)
		}
	}()

	// Connect to MongoDB (non-fatal for MVP — server starts even without DB)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var mongoClient *mongoinfra.Client
	var userRepo *mongoinfra.UserRepository
	mongoClient, err = mongoinfra.NewClient(ctx, cfg.Mongo.URI, cfg.Mongo.Database)
	if err != nil {
		logger.Warn("Failed to connect to MongoDB — server will start without database",
			zap.Error(err),
			zap.String("uri", cfg.Mongo.URI),
		)
	} else {
		defer func() {
			if err := mongoClient.Disconnect(context.Background()); err != nil {
				logger.Error("Failed to disconnect MongoDB", zap.Error(err))
			}
		}()
		logger.Info("MongoDB connected", zap.String("database", cfg.Mongo.Database))

		// Ensure indexes
		if err := mongoinfra.EnsureIndexes(ctx, mongoClient.DB()); err != nil {
			logger.Warn("Failed to ensure indexes", zap.Error(err))
		}

		// Auto-create system admin if MongoDB connected
		userRepo = mongoinfra.NewUserRepository(mongoClient.DB())
		if err := ensureSystemAdmin(ctx, userRepo, logger); err != nil {
			logger.Warn("Failed to ensure system admin", zap.Error(err))
		}
	}

	// Initialize JWT manager
	jwtManager := middleware.NewJWTManager(cfg.JWT.Secret, cfg.JWT.Expiration)

	// Initialize audit logger
	auditLogger := middleware.NewAuditLogger(mongoClient.Collection(model.CollAuditLogs))

	// ── SPEC-004: Agent Engine & Services ──

	// Initialize LLM Router with default model from env
	llmRouter := agent.NewRouter()
	if model := os.Getenv("LLM_MODEL"); model != "" {
		llmRouter.RegisterModel("default", &agent.ModelConfig{
			Model:       model,
			BaseURL:     getEnvOrDefault("LLM_BASE_URL", "https://api.openai.com"),
			APIKey:      os.Getenv("LLM_API_KEY"),
			MaxTokens:   4096,
			Temperature: 0.7,
			IsDefault:   true,
		})
	}

	// Initialize Security Auditor
	secAuditor := security.NewAuditor(nil)

	// Initialize Circuit Breaker Registry
	cbRegistry := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())

	// Initialize Session Manager (24h TTL)
	sessionManager := chat.NewManager(24 * time.Hour)

	// Initialize Skill Registry (placeholder — real skills in SPEC-008)
	skillRegistry := agent.NewSkillRegistryAdapter()

	// Initialize Agent Engine
	engine := agent.NewEngine(llmRouter, skillRegistry, secAuditor)

	// Initialize Chat Service
	chatService := chat.NewService(engine, sessionManager, secAuditor, cbRegistry)

	// Initialize Agent Service
	agentService := agent_svc.NewService(engine, chatService, sessionManager, cbRegistry)

	// ── SPEC-005: Artifact Storage & Workspace ──

	// Initialize SeaweedFS client (non-fatal — artifacts work only if available)
	swClient := seaweedfs.NewClient(cfg.SeaweedFS.Master, cfg.SeaweedFS.Filer)
	_ = swClient // ready for use

	// Initialize Artifact Storage
	artifactStorage := artifact_svc.NewStorage(swClient, mongoClient.DB())

	// Initialize Workspace Manager
	workspaceMgr := workspace.NewManager(artifactStorage)

	// Initialize Artifact HTTP Handler
	artifactHandler := handler.NewArtifactHandler(artifactStorage, workspaceMgr)

	// ── SPEC-006: Knowledge Base ──
	kbService := knowledge.NewService(mongoClient.DB())
	kbHandler := handler.NewKnowledgeHandler(kbService)

	// ── Setup Gin Router ──
	if cfg.Log.Level != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Global middleware
	router.Use(middleware.CORSMiddleware())
	router.Use(middleware.RequestIDMiddleware())
	router.Use(gin.Recovery())
	router.Use(auditLogger.AuditMiddleware())

	// Health check (no auth required)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
	})

	// Auth routes (no auth required)
	auth := router.Group("/api/v1/auth")
	auth.POST("/login", func(c *gin.Context) {
		// TODO: Implement login in SPEC-004 (needs auth handler + service)
		c.JSON(http.StatusOK, gin.H{"message": "login endpoint placeholder"})
	})

	// Protected routes
	api := router.Group("/api/v1")
	api.Use(jwtManager.AuthMiddleware())

	// User management (requires user:manage or user:manage_all)
	api.GET("/users", middleware.RequirePermission("user:manage"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "users list placeholder"})
	})

	api.GET("/users/:id", middleware.RequirePermission("user:manage"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "user detail placeholder"})
	})

	// Admin-only routes
	admin := router.Group("/api/v1/admin")
	admin.Use(jwtManager.AuthMiddleware())
	admin.GET("/dashboard", middleware.RequirePermission("system:config"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "admin dashboard placeholder"})
	})

	// ── SPEC-004: Chat & Agent routes ──

	// Chat endpoint (streaming SSE)
	chatRoutes := router.Group("/api/v1/chat")
	chatRoutes.Use(jwtManager.AuthMiddleware())
	chatRoutes.POST("", agentService.HandleChat)

	// Agent endpoints
	agentRoutes := router.Group("/api/v1/agent")
	agentRoutes.Use(jwtManager.AuthMiddleware())
	agentRoutes.POST("/tasks", agentService.CreateAgentTask)
	agentRoutes.GET("/tasks/:task_id", agentService.GetAgentTask)
	agentRoutes.GET("/skills", agentService.ListSkills)
	agentRoutes.GET("/skills/search", agentService.SearchSkills)

	// Session management
	sessionRoutes := router.Group("/api/v1/sessions")
	sessionRoutes.Use(jwtManager.AuthMiddleware())
	sessionRoutes.POST("", func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		sess, err := sessionManager.Create(userID.(string), "chat")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{
			"session_id":  sess.ID,
			"expires_at":  sess.ExpiresAt,
		})
	})

	// ── SPEC-005: Artifact routes ──
	artifactRoutes := router.Group("/api/v1/artifacts")
	artifactRoutes.Use(jwtManager.AuthMiddleware())
	artifactRoutes.POST("/upload", artifactHandler.Upload)
	artifactRoutes.GET("/:id/download", artifactHandler.Download)
	artifactRoutes.DELETE("/:id", artifactHandler.Delete)
	artifactRoutes.GET("", artifactHandler.ListSession)

	// Workspace routes
	wsRoutes := router.Group("/api/v1/workspace/:session_id")
	wsRoutes.Use(jwtManager.AuthMiddleware())
	wsRoutes.GET("/files", artifactHandler.ListWorkspace)
	wsRoutes.GET("/files/:filename", artifactHandler.ReadWorkspaceFile)
	wsRoutes.PUT("/files/:filename", artifactHandler.WriteWorkspaceFile)

	// ── SPEC-006: Knowledge Base routes ──
	kbRoutes := router.Group("/api/v1/knowledge")
	kbRoutes.Use(jwtManager.AuthMiddleware())
	kbRoutes.POST("/docs", kbHandler.UploadDoc)
	kbRoutes.GET("/docs", kbHandler.ListDocs)
	kbRoutes.GET("/docs/:id", kbHandler.GetDoc)
	kbRoutes.DELETE("/docs/:id", kbHandler.DeleteDoc)
	kbRoutes.POST("/docs/:id/chunks", kbHandler.AddChunks)
	kbRoutes.GET("/search", kbHandler.Search)

	// Start server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Graceful shutdown
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		logger.Info("Shutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			logger.Fatal("Server forced to shutdown", zap.Error(err))
		}
	}()

	logger.Info("DataAgent server starting",
		zap.Int("port", cfg.Server.Port),
		zap.String("log_level", cfg.Log.Level),
	)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal("Server error", zap.Error(err))
	}

	logger.Info("Server exited gracefully")
}

// initLogger initializes a structured logger based on config.
func initLogger(cfg *config.Config) (*zap.Logger, error) {
	var zapCfg zap.Config
	if cfg.Log.Format == "json" {
		zapCfg = zap.NewProductionConfig()
	} else {
		zapCfg = zap.NewDevelopmentConfig()
	}

	switch cfg.Log.Level {
	case "debug":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		zapCfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	return zapCfg.Build()
}

// ensureSystemAdmin creates the system_admin user if none exists.
func ensureSystemAdmin(ctx context.Context, repo *mongoinfra.UserRepository, logger *zap.Logger) error {
	hasAdmin, err := repo.HasSystemAdmin(ctx)
	if err != nil {
		return fmt.Errorf("check system admin: %w", err)
	}

	if hasAdmin {
		logger.Info("System admin already exists, skipping auto-creation")
		return nil
	}

	password, err := middleware.GenerateRandomPassword(16)
	if err != nil {
		return fmt.Errorf("generate admin password: %w", err)
	}

	passwordHash, err := middleware.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	admin := &model.User{
		Username:        "系统管理员",
		PasswordHash:    passwordHash,
		Role:            model.RoleSystemAdmin,
		PasswordChanged: false,
	}

	if err := repo.Create(ctx, admin); err != nil {
		return fmt.Errorf("create system admin: %w", err)
	}

	logger.Info("SYSTEM ADMIN AUTO-CREATED",
		zap.String("username", admin.Username),
		zap.String("password", password),
		zap.String("role", string(model.RoleSystemAdmin)),
		zap.String("note", "请尽快修改密码！登录后横幅提示修改"),
	)

	return nil
}

// getEnvOrDefault returns the env var value or a default.
func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
