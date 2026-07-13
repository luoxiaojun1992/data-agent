// Package main is the entry point for the DataAgent server.
// SPEC-003: Infrastructure setup with full middleware stack.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
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
	apireview "github.com/luoxiaojun1992/data-agent/internal/service/apireview"
	artifact_svc "github.com/luoxiaojun1992/data-agent/internal/service/artifact"
	auditsvc "github.com/luoxiaojun1992/data-agent/internal/service/audit"
	"github.com/luoxiaojun1992/data-agent/internal/api/handler"
	"github.com/luoxiaojun1992/data-agent/internal/service/chat"
	"github.com/luoxiaojun1992/data-agent/internal/service/knowledge"
	task_svc "github.com/luoxiaojun1992/data-agent/internal/service/task"
	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	mongoinfra "github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
	"github.com/luoxiaojun1992/data-agent/internal/infra/redis"
	"github.com/luoxiaojun1992/data-agent/internal/infra/seaweedfs"
	vaultinfra "github.com/luoxiaojun1992/data-agent/internal/infra/vault"
	"github.com/luoxiaojun1992/data-agent/internal/logic/workspace"
	"github.com/luoxiaojun1992/data-agent/internal/queue"
	"github.com/luoxiaojun1992/data-agent/internal/scheduler"
	"github.com/luoxiaojun1992/data-agent/internal/service/im"
	"github.com/luoxiaojun1992/data-agent/internal/service/monitor"
	notifsvc "github.com/luoxiaojun1992/data-agent/internal/service/notification"
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
	var roleRepo *mongoinfra.RoleRepository
	var systemConfigRepo *mongoinfra.SystemConfigRepository
	var vaultClient *vaultinfra.Client
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

		// Initialize role repo
		roleRepo = mongoinfra.NewRoleRepository(mongoClient.DB())

		// Initialize system config repo
		systemConfigRepo = mongoinfra.NewSystemConfigRepository(mongoClient.DB())
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

	// Initialize HashiCorp Vault client
	vaultClient, err = vaultinfra.NewClient()
	if err != nil {
		logger.Warn("Failed to initialize HashiCorp Vault client — API key encryption disabled",
			zap.Error(err),
			zap.String("VAULT_ADDR", vaultinfra.GetAddr()),
		)
	} else {
		logger.Info("HashiCorp Vault client initialized",
			zap.String("addr", vaultinfra.GetAddr()),
		)
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

	// ── Audit Log Service ──
	auditService := auditsvc.NewService(mongoClient.DB())
	auditHandler := handler.NewAuditHandler(auditService)

	// ── API Review Service ──
	apiReviewSvc := apireview.NewService(mongoClient.DB())
	apiReviewHandler := handler.NewAPIReviewHandler(apiReviewSvc)

	// ── Notification Service ──
	notifSvc := notifsvc.NewService(mongoClient.DB())
	notifHandler := handler.NewNotificationHandler(notifSvc)

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

	// ── SPEC-012: Hermes Agent Proxy (nousresearch/hermes-agent) ──
	hermesURL := os.Getenv("HERMES_URL")
	if hermesURL != "" {
		router.Any("/api/v1/hermes/*path", func(c *gin.Context) {
			target, _ := url.Parse(hermesURL)
			p := httputil.NewSingleHostReverseProxy(target)
			c.Request.URL.Path = c.Param("path")
			p.ServeHTTP(c.Writer, c.Request)
		})
		logger.Info("Hermes proxy enabled", zap.String("hermes_url", hermesURL))
	}

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
			"status":   user.Status,
		})
	})

	// POST /users — Create user (system_admin or admin)
	api.POST("/users", middleware.RequirePermission("user:manage"), func(c *gin.Context) {
		if userRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
			return
		}
		var req struct {
			Username string          `json:"username"`
			Password string          `json:"password"`
			Role     model.UserRole  `json:"role"`
			Status   model.UserStatus `json:"status"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
			return
		}
		if req.Username == "" || req.Password == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
			return
		}
		if req.Role == "" {
			req.Role = model.RoleUser
		}
		if req.Status == "" {
			req.Status = model.StatusEnabled
		}

		// System admin uniqueness check
		if req.Role == model.RoleSystemAdmin {
			hasAdmin, err := userRepo.HasSystemAdmin(c.Request.Context())
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			if hasAdmin {
				c.JSON(http.StatusConflict, gin.H{"error": "系统管理员已存在，无法创建"})
				return
			}
		}

		// Email uniqueness check
		existing, err := userRepo.FindByUsername(c.Request.Context(), req.Username)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if existing != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "该邮箱已被注册"})
			return
		}

		hash, err := middleware.HashPassword(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
			return
		}

		user := &model.User{
			Username:     req.Username,
			PasswordHash: hash,
			Role:         req.Role,
			Status:       req.Status,
		}
		if err := userRepo.Create(c.Request.Context(), user); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{
			"id":       user.ID.Hex(),
			"username": user.Username,
			"role":     user.Role,
			"status":   user.Status,
		})
	})

	// PUT /users/:id — Update user role
	api.PUT("/users/:id", middleware.RequirePermission("user:manage"), func(c *gin.Context) {
		if userRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
			return
		}
		userID := c.Param("id")
		user, err := userRepo.FindByID(c.Request.Context(), userID)
		if err != nil || user == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		// Prevent downgrading system_admin
		if user.Role == model.RoleSystemAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "不能修改系统管理员的角色"})
			return
		}
		var req struct {
			Role model.UserRole `json:"role"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		if req.Role != model.RoleSystemAdmin && req.Role != model.RoleAdmin && req.Role != model.RoleUser {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
			return
		}
		if err := userRepo.UpdateRole(c.Request.Context(), userID, req.Role); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	// PATCH /users/:id/status — Toggle user enable/disable
	api.PATCH("/users/:id/status", middleware.RequirePermission("user:manage"), func(c *gin.Context) {
		if userRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
			return
		}
		userID := c.Param("id")
		user, err := userRepo.FindByID(c.Request.Context(), userID)
		if err != nil || user == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		// Prevent disabling system_admin
		if user.Role == model.RoleSystemAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "不能停用系统管理员"})
			return
		}
		var req struct {
			Status model.UserStatus `json:"status"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		if err := userRepo.UpdateStatus(c.Request.Context(), userID, req.Status); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	// DELETE /users/:id — Delete user
	api.DELETE("/users/:id", middleware.RequirePermission("user:manage"), func(c *gin.Context) {
		if userRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
			return
		}
		userID := c.Param("id")
		user, err := userRepo.FindByID(c.Request.Context(), userID)
		if err != nil || user == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		// Prevent deleting system_admin
		if user.Role == model.RoleSystemAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "不可删除系统管理员"})
			return
		}
		if err := userRepo.Delete(c.Request.Context(), userID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	// ── Role management ──

	// GET /roles — List all roles (fixed + custom)
	api.GET("/roles", middleware.RequirePermission("user:manage"), func(c *gin.Context) {
		customRoles := []model.Role{}
		if roleRepo != nil {
			var err error
			customRoles, err = roleRepo.List(c.Request.Context())
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		// Merge fixed roles (from code) with custom roles (from DB)
		fixedRoles := model.FixedRoles()
		allRoles := append(fixedRoles, customRoles...)
		c.JSON(http.StatusOK, gin.H{"roles": allRoles, "total": len(allRoles)})
	})

	// GET /permissions — List all available permissions with metadata
	api.GET("/permissions", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"permissions": model.GetAllPermissions()})
	})

	// POST /roles — Create custom role
	api.POST("/roles", middleware.RequirePermission("user:manage"), func(c *gin.Context) {
		if roleRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
			return
		}
		var req struct {
			Name        string   `json:"name"`
			DisplayName string   `json:"display_name"`
			Permissions []string `json:"permissions"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		if req.Name == "" || req.DisplayName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name and display_name are required"})
			return
		}
		role := &model.Role{
			Name:        req.Name,
			DisplayName: req.DisplayName,
			Permissions: req.Permissions,
			Type:        "custom",
		}
		if len(role.Permissions) == 0 {
			role.Permissions = []string{}
		}
		if err := roleRepo.Create(c.Request.Context(), role); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{
			"id":          role.ID.Hex(),
			"name":        role.Name,
			"display_name": role.DisplayName,
			"permissions": role.Permissions,
			"type":        role.Type,
		})
	})

	// PUT /roles/:id — Update custom role permissions
	api.PUT("/roles/:id", middleware.RequirePermission("user:manage"), func(c *gin.Context) {
		if roleRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
			return
		}
		roleID := c.Param("id")
		role, err := roleRepo.FindByID(c.Request.Context(), roleID)
		if err != nil || role == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
			return
		}
		var req struct {
			Permissions []string `json:"permissions"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		if err := roleRepo.Update(c.Request.Context(), roleID, req.Permissions); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	// DELETE /roles/:id — Delete custom role (fixed roles blocked)
	api.DELETE("/roles/:id", middleware.RequirePermission("user:manage"), func(c *gin.Context) {
		if roleRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
			return
		}
		roleID := c.Param("id")
		role, err := roleRepo.FindByID(c.Request.Context(), roleID)
		if err != nil || role == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
			return
		}
		if role.Type == "fixed" {
			c.JSON(http.StatusForbidden, gin.H{"error": "不可删除固定角色"})
			return
		}
		if err := roleRepo.Delete(c.Request.Context(), roleID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	// ── Model Config & Vault ──

	// GET /model-config — Get current model configuration
	api.GET("/model-config", middleware.RequirePermission("user:manage"), func(c *gin.Context) {
		configs := gin.H{
			"api_url":        "https://api.openai.com/v1",
			"api_key_exists": false,
			"model_name":     "gpt-4o",
			"context_len":    128000,
			"max_output":     16000,
			"temperature":    0.7,
			"top_p":          0.95,
			"hermes_url":     "http://hermes:8081",
			"hermes_model":   "hermes-3-70b",
		}
		if systemConfigRepo != nil {
			dbConfigs, _ := systemConfigRepo.GetAll(c.Request.Context(), "model")
			result := gin.H{}
			for _, cfg := range dbConfigs {
				if cfg.Key == "api_key" || cfg.Key == "hermes_api_key" {
					result[cfg.Key] = cfg.Value // encrypted, only decrypted via /vault/decrypt
					result["api_key_exists"] = true
				} else {
					result[cfg.Key] = cfg.Value
				}
			}
			// Defaults
			if result["api_url"] == nil {
				result["api_url"] = "https://api.openai.com/v1"
			}
			if result["model_name"] == nil {
				result["model_name"] = "gpt-4o"
			}
			if result["context_len"] == nil {
				result["context_len"] = "128000"
			}
			if result["max_output"] == nil {
				result["max_output"] = "16000"
			}
			if result["temperature"] == nil {
				result["temperature"] = "0.7"
			}
			if result["top_p"] == nil {
				result["top_p"] = "0.95"
			}
			if result["hermes_url"] == nil {
				result["hermes_url"] = "http://hermes:8081"
			}
			if result["hermes_model"] == nil {
				result["hermes_model"] = "hermes-3-70b"
			}
			result["api_key_exists"] = result["api_key"] != nil && result["api_key"] != ""
			c.JSON(http.StatusOK, result)
			return
		}
		c.JSON(http.StatusOK, configs)
	})

	// PUT /model-config — Save model configuration
	api.PUT("/model-config", middleware.RequirePermission("user:manage"), func(c *gin.Context) {
		if systemConfigRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
			return
		}
		var body map[string]interface{}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		for key, val := range body {
			if valStr, ok := val.(string); ok {
				// API keys go to HashiCorp Vault, not DB
				if (key == "api_key" || key == "hermes_api_key") && valStr != "" {
					if vaultClient != nil {
						vaultPath := vaultinfra.APIKeyPath("data-agent")
						if key == "hermes_api_key" {
							vaultPath = vaultinfra.HermesAPIKeyPath("data-agent")
						}
						if err := vaultClient.Store(c.Request.Context(), vaultPath, valStr); err != nil {
							c.JSON(http.StatusInternalServerError, gin.H{"error": "vault store failed"})
							return
						}
					}
					// Store marker in DB to indicate key exists
					_ = systemConfigRepo.Upsert(c.Request.Context(), "model", key, "vault://data-agent/"+key)
				} else {
					_ = systemConfigRepo.Upsert(c.Request.Context(), "model", key, valStr)
				}
			}
		}
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	// POST /vault/decrypt — Retrieve API key from HashiCorp Vault
	api.POST("/vault/decrypt", middleware.RequirePermission("user:manage"), func(c *gin.Context) {
		if vaultClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "vault not configured"})
			return
		}
		if !vaultClient.IsAvailable(c.Request.Context()) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "vault service unavailable"})
			return
		}
		var req struct {
			Key string `json:"key"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		if req.Key == "" {
			req.Key = vaultinfra.APIKeyPath("data-agent")
		}
		plaintext, err := vaultClient.Retrieve(c.Request.Context(), req.Key)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "vault retrieve failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"plaintext": plaintext, "masked": vaultinfra.MaskValue(plaintext)})
	})

	// ── System Config ──

	// GET /sysconfig — Get all system configuration
	api.GET("/sysconfig", middleware.RequirePermission("system:config"), func(c *gin.Context) {
		if systemConfigRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
			return
		}
		dbConfigs, _ := systemConfigRepo.GetAll(c.Request.Context(), "sys")
		result := gin.H{
			"session_recovery_hours":    24,
			"audit_retention_days":      90,
			"notification_ttl_days":     90,
			"email_whitelist":           []string{},
			"report_retry_count":        3,
		}
		for _, cfg := range dbConfigs {
			result[cfg.Key] = cfg.Value
		}
		// Parse email_whitelist if stored as comma-separated string
		if s, ok := result["email_whitelist"].(string); ok && s != "" {
			result["email_whitelist"] = strings.Split(s, ",")
		}
		c.JSON(http.StatusOK, result)
	})

	// PUT /sysconfig — Save system configuration values
	api.PUT("/sysconfig", middleware.RequirePermission("system:config"), func(c *gin.Context) {
		if systemConfigRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
			return
		}
		var body map[string]interface{}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}

		// Validation: session recovery hours max 168
		if hours, ok := body["session_recovery_hours"]; ok {
			if h, ok := toFloat64(hours); ok && (h < 1 || h > 168) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "缓冲期最长 1 周（168 小时）"})
				return
			}
		}

		// Store each key-value pair
		for key, val := range body {
			if list, ok := val.([]interface{}); ok {
				// Join list as comma-separated string for email_whitelist
				parts := make([]string, len(list))
				for i, v := range list {
					parts[i] = fmt.Sprintf("%v", v)
				}
				_ = systemConfigRepo.Upsert(c.Request.Context(), "sys", key, strings.Join(parts, ","))
			} else {
				_ = systemConfigRepo.Upsert(c.Request.Context(), "sys", key, fmt.Sprintf("%v", val))
			}
		}
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
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

	// Admin KB management (global view)
	adminKB := router.Group("/api/v1/admin/knowledge")
	adminKB.Use(jwtManager.AuthMiddleware(), middleware.RequirePermission("user:manage"))
	adminKB.GET("/docs", kbHandler.ListAllDocs)

	// ── Audit Log routes (admin only) ──
	auditRoutes := router.Group("/api/v1/admin/audit")
	auditRoutes.Use(jwtManager.AuthMiddleware(), middleware.RequirePermission("audit:view"))
	auditRoutes.GET("/logs", auditHandler.ListAuditLogs)
	auditRoutes.POST("/export", auditHandler.ExportAuditLogs)

	// ── API Review routes (admin only) ──
	apiRevRoutes := router.Group("/api/v1/admin/api-reviews")
	apiRevRoutes.Use(jwtManager.AuthMiddleware(), middleware.RequirePermission("api:convert"))
	apiRevRoutes.GET("", apiReviewHandler.ListAPIReviews)
	apiRevRoutes.POST("", apiReviewHandler.CreateAPIReview)
	apiRevRoutes.PUT("/:id/approve", apiReviewHandler.ApproveAPIReview)
	apiRevRoutes.PUT("/:id/reject", apiReviewHandler.RejectAPIReview)

	// ── Notification routes ──
	notifRoutes := router.Group("/api/v1/notifications")
	notifRoutes.Use(jwtManager.AuthMiddleware())
	notifRoutes.GET("", notifHandler.ListNotifications)
	notifRoutes.GET("/unread-count", notifHandler.UnreadCount)
	notifRoutes.PUT("/:id/read", notifHandler.MarkRead)
	notifRoutes.PUT("/read-all", notifHandler.MarkAllRead)
	notifRoutes.POST("", notifHandler.SendNotification)
	notifRoutes.POST("/broadcast", notifHandler.BroadcastNotification)

	// ── SPEC-009: Task routes ──
	if taskHandler != nil {
		taskRoutes := router.Group("/api/v1/tasks")
		taskRoutes.Use(jwtManager.AuthMiddleware())
		taskRoutes.POST("", taskHandler.CreateTask)
		taskRoutes.GET("", taskHandler.ListTasks)
		taskRoutes.GET("/:task_id", taskHandler.GetTask)
		taskRoutes.PUT("/:task_id/cancel", taskHandler.CancelTask)
		taskRoutes.PUT("/:task_id/pause", taskHandler.PauseTask)
		taskRoutes.PUT("/:task_id/resume", taskHandler.ResumeTask)
		taskRoutes.GET("/:task_id/artifacts/download", taskHandler.DownloadArtifacts)

		// Admin task management (global view)
		adminTasks := router.Group("/api/v1/admin/tasks")
		adminTasks.Use(jwtManager.AuthMiddleware(), middleware.RequirePermission("user:manage"))
		adminTasks.GET("", taskHandler.ListAllTasks)
		adminTasks.PUT("/:task_id/retry", taskHandler.RetryTask)
		adminTasks.POST("/batch-cancel", taskHandler.BatchCancelTasks)
	}

	// ── Dashboard stats endpoint ──
	router.GET("/api/v1/dashboard", jwtManager.AuthMiddleware(), func(c *gin.Context) {
		stats := monitor.SystemStats()
		userID, _ := c.Get("user_id")

		// Query real task data
		taskStats := map[string]int{"total": 0, "pending": 0, "running": 0, "completed": 0, "failed": 0}
		sessionCount := 0
		docCount := 0

		if taskHandler != nil {
			userIDStr := userID.(string)
			// Task counts by status
			if taskService != nil {
				tasks, err := taskService.ListTasks(userIDStr)
				if err == nil {
					for _, t := range tasks {
						taskStats["total"]++
						switch string(t.Status) {
						case "pending": taskStats["pending"]++
						case "running": taskStats["running"]++
						case "completed": taskStats["completed"]++
						case "failed": taskStats["failed"]++
						}
					}
				}
			}
		}

		// Session count
		userSessions := sessionManager.ListByUser(userID.(string))
		sessionCount = len(userSessions)

		// KB doc count
		if kbService != nil {
			docs, err := kbService.ListDocs(userID.(string))
			if err == nil {
				docCount = len(docs)
			}
		}

		stats["kpis"] = []map[string]interface{}{
			{"label": "活跃 Chat 会话", "value": sessionCount, "icon": "💬", "trend": "实时"},
			{"label": "Agent 任务", "value": taskStats["total"], "icon": "⚡", "trend": "实时"},
			{"label": "知识库文档", "value": docCount, "icon": "📚", "trend": "实时"},
			{"label": "系统可用率", "value": "99.9%", "icon": "🟢", "trend": "稳定"},
		}
		stats["task_stats"] = taskStats

		c.JSON(http.StatusOK, stats)
	})

	// ── Dashboard trends (time-series) endpoint ──
	router.GET("/api/v1/dashboard/trends", jwtManager.AuthMiddleware(), func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		var allTasks []task.Task

		if taskService != nil {
			allTasks, _ = taskService.ListTasks(userID.(string))
		}

		var docs int
		userSessions := sessionManager.ListByUser(userID.(string))
		if kbService != nil {
			d, err := kbService.ListDocs(userID.(string))
			if err == nil {
				docs = len(d)
			}
		}

		trends := monitor.ComputeTrends(allTasks, make([]interface{}, len(userSessions)), docs)
		c.JSON(http.StatusOK, trends)
	})

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
// toFloat64 tries to convert an interface{} to float64.
func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case json.Number:
		f, err := val.Float64()
		return f, err == nil
	}
	return 0, false
}

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
