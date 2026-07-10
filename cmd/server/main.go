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
	"github.com/luoxiaojun1992/data-agent/internal/domain/skill"
	authsvc "github.com/luoxiaojun1992/data-agent/internal/service/auth"
	kbskill "github.com/luoxiaojun1992/data-agent/skills/knowledge_search"
	saveskill "github.com/luoxiaojun1992/data-agent/skills/save_report"
	sqlskill "github.com/luoxiaojun1992/data-agent/skills/sql_executor"
	statsskill "github.com/luoxiaojun1992/data-agent/skills/stats_engine"
	agent_svc "github.com/luoxiaojun1992/data-agent/internal/service/agent"
	artifact_svc "github.com/luoxiaojun1992/data-agent/internal/service/artifact"
	"github.com/luoxiaojun1992/data-agent/internal/api/handler"
	"github.com/luoxiaojun1992/data-agent/internal/service/chat"
	"github.com/luoxiaojun1992/data-agent/internal/service/knowledge"
	task_svc "github.com/luoxiaojun1992/data-agent/internal/service/task"
	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	mongoinfra "github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
	"github.com/luoxiaojun1992/data-agent/internal/infra/redis"
	"github.com/luoxiaojun1992/data-agent/internal/infra/seaweedfs"
	"github.com/luoxiaojun1992/data-agent/internal/logic/workspace"
	"github.com/luoxiaojun1992/data-agent/internal/queue"
	"github.com/luoxiaojun1992/data-agent/internal/scheduler"
	"github.com/luoxiaojun1992/data-agent/internal/service/im"
	"github.com/luoxiaojun1992/data-agent/internal/service/monitor"
	"github.com/luoxiaojun1992/data-agent/internal/worker"
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

	// Initialize auth service and handler
	var authHandler *handler.AuthHandler
	if userRepo != nil {
		authService := authsvc.NewService(userRepo, jwtManager)
		authHandler = handler.NewAuthHandler(authService)
	}

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

	// Initialize Skill Registry — all skills are registered at startup
	skillRegistry := skill.NewRegistry()
	if err := skillRegistry.Register(&sqlskill.SQLExecutor{}); err != nil {
		logger.Warn("Failed to register sql_executor skill", zap.Error(err))
	}
	if err := skillRegistry.Register(&statsskill.StatsEngine{}); err != nil {
		logger.Warn("Failed to register stats_engine skill", zap.Error(err))
	}
	if err := skillRegistry.Register(&saveskill.SaveReport{}); err != nil {
		logger.Warn("Failed to register save_report skill", zap.Error(err))
	}
	// knowledge_search requires the KB service (registered after kbService init)

	// Initialize Agent Engine with skill registry
	engine := agent.NewEngine(llmRouter, agent.NewSkillRegistryFromDomain(skillRegistry), secAuditor)

	// Initialize Chat Service
	chatService := chat.NewService(engine, sessionManager, secAuditor, cbRegistry)

	// Initialize Agent Service
	agentService := agent_svc.NewService(engine, chatService, sessionManager, cbRegistry)
	agentService.WithSkillRegistry(agent.NewSkillRegistryFromDomain(skillRegistry))

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

	// Register knowledge search skill (requires kbService)
	if err := skillRegistry.Register(kbskill.NewKnowledgeSearch(kbService)); err != nil {
		logger.Warn("Failed to register knowledge_search skill", zap.Error(err))
	}

	// ── SPEC-009: Task Queue & Worker Pool ──

	var taskHandler *handler.TaskHandler
	var taskService *task_svc.Service

	redisClient, redisErr := redis.NewClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if redisErr != nil {
		logger.Warn("Failed to connect to Redis — task queue disabled", zap.Error(redisErr))
	} else {
		defer redisClient.Close()

		taskStream, streamErr := queue.NewStream(redisClient.Client())
		if streamErr != nil {
			logger.Warn("Failed to create task stream", zap.Error(streamErr))
		} else {
		taskService = task_svc.NewService(mongoClient.DB(), taskStream)
		taskHandler = handler.NewTaskHandler(taskService)

		// Inject task service into agent service for Redis-backed async tasks
		agentService.WithTaskService(taskService)

		// Initialize Scheduler
		sched := scheduler.New(scheduler.NewTaskCreatorFromService(taskService))
		sched.Start(context.Background())

		// Register default scheduled tasks
		if err := sched.AddSchedule(&scheduler.Schedule{
			Name:       "System Monitoring Stats",
			CronExpr:   "every_5m",
			Enabled:    true,
			SkillChain: []string{"stats_engine"},
			Params:     map[string]interface{}{"method": "descriptive"},
		}); err != nil {
			logger.Warn("Failed to add monitoring schedule", zap.Error(err))
		}
		logger.Info("Scheduler started with default tasks")

			workerPool := worker.NewPool(taskStream, redisClient.Client(), 4, &simpleExecutor{taskSvc: taskService})
			workerPool.Start(context.Background())
			defer workerPool.Stop()

			logger.Info("Task queue and worker pool started", zap.Int("workers", 4))
		}
	}
	_ = taskService // used for route registration check

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

	// ── SPEC-011: Feishu IM Webhook (no auth) ──
	imService := im.NewService(im.Config{
		AppID:     os.Getenv("FEISHU_APP_ID"),
		AppSecret: os.Getenv("FEISHU_APP_SECRET"),
	})
	router.POST("/api/v1/im/feishu/webhook", func(c *gin.Context) {
		imService.WebhookHandler()(c.Writer, c.Request)
	})

	// ── SPEC-012: Hermes Free Explore (独立服务，不再路由到主二进制) ──
	// Hermes 现在是独立二进制 cmd/hermes/main.go，通过 Docker Compose 独立部署
	// 如果需要代理模式，设置 HERMES_URL 环境变量后取消下方注释
	// hermesURL := os.Getenv("HERMES_URL")
	// hermesSvc := hermes.NewService(hermesURL)
	// router.Any("/api/v1/hermes/*path", func(c *gin.Context) {
	// 	hermesSvc.Proxy(c.Writer, c.Request)
	// })

	// ── SPEC-010: System Monitoring ──
	router.GET("/api/v1/system/stats", monitor.Handler())

	// Auth routes (no auth required)
	authGroup := router.Group("/api/v1/auth")
	if authHandler != nil {
		authGroup.POST("/login", authHandler.Login)
		authGroup.POST("/register", authHandler.Register)
	} else {
		authGroup.POST("/login", func(c *gin.Context) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
		})
		authGroup.POST("/register", func(c *gin.Context) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
		})
	}

	// Protected routes
	api := router.Group("/api/v1")
	api.Use(jwtManager.AuthMiddleware())

	// Token refresh (requires valid JWT)
	api.POST("/auth/refresh", func(c *gin.Context) {
		if authHandler != nil {
			authHandler.RefreshToken(c)
		} else {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
		}
	})

	// Profile
	api.GET("/auth/profile", func(c *gin.Context) {
		if authHandler != nil {
			authHandler.GetProfile(c)
		} else {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
		}
	})

	// User management (requires user:manage or user:manage_all)
	api.GET("/users", middleware.RequirePermission("user:manage"), func(c *gin.Context) {
		if userRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
			return
		}
		role, _ := c.Get("role")
		users, total, err := userRepo.List(c.Request.Context(), role.(string), 0, 100)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"users": users, "total": total})
	})

	api.GET("/users/:id", middleware.RequirePermission("user:manage"), func(c *gin.Context) {
		if userRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
			return
		}
		user, err := userRepo.FindByID(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if user == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"id":       user.ID.Hex(),
			"username": user.Username,
			"role":     user.Role,
		})
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

	// List user sessions
	sessionRoutes.GET("", func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		sessions := sessionManager.ListByUser(userID.(string))
		c.JSON(http.StatusOK, gin.H{"sessions": sessions})
	})

	// Delete session
	sessionRoutes.DELETE("/:id", func(c *gin.Context) {
		if err := sessionManager.Delete(c.Param("id")); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "deleted"})
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

	// ── SPEC-009: Task routes ──
	if taskHandler != nil {
		taskRoutes := router.Group("/api/v1/tasks")
		taskRoutes.Use(jwtManager.AuthMiddleware())
		taskRoutes.POST("", taskHandler.CreateTask)
		taskRoutes.GET("", taskHandler.ListTasks)
		taskRoutes.GET("/:task_id", taskHandler.GetTask)
		taskRoutes.PUT("/:task_id/cancel", taskHandler.CancelTask)
	}

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

// simpleExecutor is a basic task executor for the worker pool.
// In production, this would route to the Agent Engine for actual execution.
type simpleExecutor struct {
	taskSvc *task_svc.Service
}

func (e *simpleExecutor) Execute(ctx context.Context, t *task.Task) error {
	// Placeholder: in production, this routes to Agent Engine
	_ = ctx
	_ = t
	return nil
}
