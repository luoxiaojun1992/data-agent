// Package main is the entry point for the DataAgent server.
// SPEC-003: Infrastructure setup with full middleware stack.
package main

import (
	"bytes"
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
	adkmemory "github.com/luoxiaojun1992/data-agent/internal/adk/memory"
	adkmodel "github.com/luoxiaojun1992/data-agent/internal/adk/model"
	adkruntime "github.com/luoxiaojun1992/data-agent/internal/adk/runtime"
	adksession "github.com/luoxiaojun1992/data-agent/internal/adk/session"
	adktools "github.com/luoxiaojun1992/data-agent/internal/adk/tools"
	"github.com/luoxiaojun1992/data-agent/internal/api/handler"
	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/config"
	"github.com/luoxiaojun1992/data-agent/internal/domain/consts"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	mongoinfra "github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
	"github.com/luoxiaojun1992/data-agent/internal/infra/redis"
	"github.com/luoxiaojun1992/data-agent/internal/infra/seaweedfs"
	vaultinfra "github.com/luoxiaojun1992/data-agent/internal/infra/vault"
	"github.com/luoxiaojun1992/data-agent/internal/logic"
	"github.com/luoxiaojun1992/data-agent/internal/logic/workspace"
	"github.com/luoxiaojun1992/data-agent/internal/queue"
	"github.com/luoxiaojun1992/data-agent/internal/scheduler"
	agent_svc "github.com/luoxiaojun1992/data-agent/internal/service/agent"
	apireview "github.com/luoxiaojun1992/data-agent/internal/service/apireview"
	artifact_svc "github.com/luoxiaojun1992/data-agent/internal/service/artifact"
	auditsvc "github.com/luoxiaojun1992/data-agent/internal/service/audit"
	authsvc "github.com/luoxiaojun1992/data-agent/internal/service/auth"
	"github.com/luoxiaojun1992/data-agent/internal/service/chat"
	"github.com/luoxiaojun1992/data-agent/internal/service/im"
	"github.com/luoxiaojun1992/data-agent/internal/service/knowledge"
	"github.com/luoxiaojun1992/data-agent/internal/service/monitor"
	notifsvc "github.com/luoxiaojun1992/data-agent/internal/service/notification"
	task_svc "github.com/luoxiaojun1992/data-agent/internal/service/task"
	"github.com/luoxiaojun1992/data-agent/internal/worker"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
	adkmodelface "google.golang.org/adk/model"
	adksessionIF "google.golang.org/adk/session"
)

// appName namespaces ADK sessions, memory entries, and tool registration.
const appName = "data-agent"

// ===================== main =====================

func main() {
	cfg, logger, mongoClient, deps := initServer()
	defer cleanup(logger, mongoClient, &deps)

	router := buildRouter(cfg)
	registerAllRoutes(router, &deps, logger)
	startServer(router, cfg, logger)
}

// ===================== server lifecycle =====================

type serverDependencies struct {
	mongoClient      *mongoinfra.Client
	userRepo         *mongoinfra.UserRepository
	roleRepo         *mongoinfra.RoleRepository
	systemConfigRepo *mongoinfra.SystemConfigRepository
	vaultClient      *vaultinfra.Client
	authHandler      *handler.AuthHandler
	adkModel         adkmodelface.LLM
	adkRuntime       *adkruntime.Runtime
	adkSessions      *adksession.Service
	memoryService    *adkmemory.Service
	sessionManager   *chat.Manager
	chatService      *chat.Service
	agentService     *agent_svc.Service
	secAuditor       *security.Auditor
	cbRegistry       *security.CircuitBreakerRegistry
	kbService        *knowledge.Service
	kbHandler        *handler.KnowledgeHandler
	artifactStorage  *artifact_svc.Storage
	workspaceMgr     *workspace.Manager
	artifactHandler  *handler.ArtifactHandler
	taskService      *task_svc.Service
	taskHandler      *handler.TaskHandler
	auditService     *auditsvc.Service
	auditHandler     *handler.AuditHandler
	apiReviewSvc     *apireview.Service
	apiReviewHandler *handler.APIReviewHandler
	notifSvc         *notifsvc.Service
	notifHandler     *handler.NotificationHandler
	auditLogger      *middleware.AuditLogger
	jwtManager       *middleware.JWTManager
	redisClient      *redis.Client
	taskStream       *queue.Stream
	swClient         *seaweedfs.Client
}

func initServer() (*config.Config, *zap.Logger, *mongoinfra.Client, serverDependencies) {
	var deps serverDependencies

	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	logger, err := initLogger(cfg)
	if err != nil {
		log.Fatalf("Failed to init logger: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var mongoClient *mongoinfra.Client
	deps.mongoClient = mongoClient
	mongoClient, err = mongoinfra.NewClient(ctx, cfg.Mongo.URI, cfg.Mongo.Database)
	deps.mongoClient = mongoClient
	if err != nil {
		logger.Warn("Failed to connect to MongoDB — server will start without database",
			zap.Error(err),
			zap.String("uri", cfg.Mongo.URI),
		)
	} else {
		logger.Info("MongoDB connected", zap.String("database", cfg.Mongo.Database))
		if err := mongoinfra.EnsureIndexes(ctx, mongoClient.DB()); err != nil {
			logger.Warn("Failed to ensure indexes", zap.Error(err))
		}
		deps.userRepo = mongoinfra.NewUserRepository(mongoClient.DB())
		if err := ensureSystemAdmin(ctx, deps.userRepo, logger); err != nil {
			logger.Warn("Failed to ensure system admin", zap.Error(err))
		}
		deps.roleRepo = mongoinfra.NewRoleRepository(mongoClient.DB())
		deps.systemConfigRepo = mongoinfra.NewSystemConfigRepository(mongoClient.DB())
	}

	deps.jwtManager = middleware.NewJWTManager(cfg.JWT.Secret, cfg.JWT.Expiration)
	deps.auditLogger = middleware.NewAuditLogger(mongoClient.Collection(model.CollAuditLogs))

	// SeaweedFS must be initialized before initArtifacts
	deps.swClient = seaweedfs.NewClient(cfg.SeaweedFS.Master, cfg.SeaweedFS.Filer)

	initAuthService(&deps, mongoClient, logger)
	initADKModel(&deps)
	initVault(&deps, logger)
	initAgentEngine(&deps)
	initKnowledgeBase(&deps, mongoClient)
	initServices(&deps, mongoClient, logger)
	initArtifacts(&deps, mongoClient, cfg)
	initAuditAndNotifications(&deps, mongoClient)
	initTaskQueue(&deps, cfg, mongoClient, logger)

	return cfg, logger, mongoClient, deps
}

func cleanup(logger *zap.Logger, mongoClient *mongoinfra.Client, deps *serverDependencies) {
	if logger != nil {
		if err := logger.Sync(); err != nil {
			log.Printf("logger sync error: %v", err)
		}
	}
	if mongoClient != nil {
		if err := mongoClient.Disconnect(context.Background()); err != nil {
			logger.Error("Failed to disconnect MongoDB", zap.Error(err))
		}
	}
	if deps.redisClient != nil {
		deps.redisClient.Close()
	}
}

func initAuthService(deps *serverDependencies, mongoClient *mongoinfra.Client, logger *zap.Logger) {
	if deps.userRepo == nil {
		return
	}
	authService := authsvc.NewService(deps.userRepo, deps.jwtManager)
	inviteRepo := mongoinfra.NewInviteRepository(mongoClient.DB())
	authService.SetInviteRepo(inviteRepo)
	hmacSecret, err := logic.LoadInviteHMACSecret()
	if err != nil {
		logger.Warn("INVITE_HMAC_SECRET not set — invite system disabled", zap.Error(err))
	} else {
		authService.SetHMACSecret(hmacSecret)
		logger.Info("Invite HMAC secret loaded")
	}
	deps.authHandler = handler.NewAuthHandler(authService)
}

// initADKModel builds the ADK model.LLM from env config, with an optional
// fallback chain (LLM_FALLBACK_BASE_URLS, comma-separated).
func initADKModel(deps *serverDependencies) {
	modelName := getEnvOrDefault("LLM_MODEL", "mock-gpt-4o")
	primary := adkmodel.NewOpenAIModel(adkmodel.Backend{
		Model:       modelName,
		BaseURL:     getEnvOrDefault("LLM_BASE_URL", "https://api.openai.com"),
		APIKey:      os.Getenv("LLM_API_KEY"),
		MaxTokens:   4096,
		Temperature: 0.7,
	})

	backends := []adkmodelface.LLM{primary}
	if raw := os.Getenv("LLM_FALLBACK_BASE_URLS"); raw != "" {
		for _, u := range strings.Split(raw, ",") {
			u = strings.TrimSpace(u)
			if u == "" {
				continue
			}
			backends = append(backends, adkmodel.NewOpenAIModel(adkmodel.Backend{
				Model:       modelName,
				BaseURL:     u,
				APIKey:      os.Getenv("LLM_API_KEY"),
				MaxTokens:   4096,
				Temperature: 0.7,
			}))
		}
	}

	chain, err := adkmodel.NewFallbackLLM(backends...)
	if err != nil {
		deps.adkModel = primary
		return
	}
	deps.adkModel = chain
}

func initVault(deps *serverDependencies, logger *zap.Logger) {
	var err error
	deps.vaultClient, err = vaultinfra.NewClient()
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
}

func initAgentEngine(deps *serverDependencies) {
	deps.secAuditor = security.NewAuditor(nil)
	deps.cbRegistry = security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
}

func initServices(deps *serverDependencies, mongoClient *mongoinfra.Client, logger *zap.Logger) {
	deps.sessionManager = chat.NewManager(mongoClient.DB(), 24*time.Hour)

	// ADK session service (MongoDB) with LLM-summarization compaction.
	deps.adkSessions = adksession.NewService(mongoClient.DB()).WithCompaction(
		adksession.CompactionConfig{MaxEvents: 100, MaxTokens: 4000, KeepRecent: 20},
		adksession.NewLLMSummarizer(deps.adkModel),
	)

	// Long-term memory (MongoDB + embedding). Embedding is optional — without
	// EMBEDDING_BASE_URL the service degrades to keyword search.
	var embed adkmemory.EmbeddingFunc
	if baseURL := os.Getenv("EMBEDDING_BASE_URL"); baseURL != "" {
		embed = adkmemory.NewOpenAIEmbedding(adkmemory.OpenAIEmbeddingConfig{
			BaseURL: baseURL,
			Model:   getEnvOrDefault("EMBEDDING_MODEL", "nomic-embed-text"),
			APIKey:  os.Getenv("EMBEDDING_API_KEY"),
		})
	} else {
		logger.Info("EMBEDDING_BASE_URL not set — memory search falls back to keyword matching")
	}
	deps.memoryService = adkmemory.NewService(mongoClient.DB(), embed)

	// ADK tools (skills rewritten as function tools).
	toolDeps := &adktools.Deps{
		KBService: deps.kbService,
		Memory:    deps.memoryService,
		AppName:   appName,
	}
	tools, err := adktools.All(toolDeps)
	if err != nil {
		logger.Fatal("Failed to build ADK tools", zap.Error(err))
	}

	// ADK runtime (llmagent ReAct loop + runner).
	rt, err := adkruntime.New(adkruntime.Config{
		AppName:        appName,
		Model:          deps.adkModel,
		SessionService: deps.adkSessions,
		MemoryService:  deps.memoryService,
		Tools:          tools,
		Auditor:        deps.secAuditor,
	})
	if err != nil {
		logger.Fatal("Failed to build ADK runtime", zap.Error(err))
	}
	deps.adkRuntime = rt

	deps.chatService = chat.NewService(rt, deps.adkSessions, deps.sessionManager, deps.cbRegistry).
		WithMemoryWrite(func(ctx context.Context, sess adksessionIF.Session) {
			if err := deps.memoryService.AddSessionToMemory(ctx, sess); err != nil {
				logger.Warn("memory write failed", zap.Error(err))
			}
		})
	deps.agentService = agent_svc.NewService(deps.chatService, deps.sessionManager, deps.cbRegistry)
	deps.agentService.WithToolLister(agent_svc.ToolListerFunc(func() []string {
		names, err := adktools.Names(toolDeps)
		if err != nil {
			return []string{}
		}
		return names
	}))
}

func initArtifacts(deps *serverDependencies, mongoClient *mongoinfra.Client, cfg *config.Config) {
	deps.artifactStorage = artifact_svc.NewStorage(deps.swClient, mongoClient.DB())
	deps.workspaceMgr = workspace.NewManager(deps.artifactStorage)
	deps.artifactHandler = handler.NewArtifactHandler(deps.artifactStorage, deps.workspaceMgr)
}

func initKnowledgeBase(deps *serverDependencies, mongoClient *mongoinfra.Client) {
	deps.kbService = knowledge.NewService(mongoClient.DB())
	deps.kbHandler = handler.NewKnowledgeHandler(deps.kbService)
}

func initAuditAndNotifications(deps *serverDependencies, mongoClient *mongoinfra.Client) {
	deps.auditService = auditsvc.NewService(mongoClient.DB())
	deps.auditHandler = handler.NewAuditHandler(deps.auditService)
	deps.apiReviewSvc = apireview.NewService(mongoClient.DB())
	deps.apiReviewHandler = handler.NewAPIReviewHandler(deps.apiReviewSvc)
	deps.notifSvc = notifsvc.NewService(mongoClient.DB())
	deps.notifHandler = handler.NewNotificationHandler(deps.notifSvc)
}

func initTaskQueue(deps *serverDependencies, cfg *config.Config, mongoClient *mongoinfra.Client, logger *zap.Logger) {
	redisClient, redisErr := redis.NewClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if redisErr != nil {
		logger.Warn("Failed to connect to Redis — task queue disabled", zap.Error(redisErr))
		return
	}
	deps.redisClient = redisClient

	taskStream, streamErr := queue.NewStream(redisClient.Client())
	if streamErr != nil {
		logger.Warn("Failed to create task stream", zap.Error(streamErr))
		return
	}
	deps.taskStream = taskStream

	deps.taskService = task_svc.NewService(mongoClient.DB(), taskStream)
	deps.taskHandler = handler.NewTaskHandler(deps.taskService)
	deps.agentService.WithTaskService(deps.taskService)

	sched := scheduler.New(scheduler.NewTaskCreatorFromService(deps.taskService))
	sched.Start(context.Background())
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

	workerPool := worker.NewPool(taskStream, redisClient.Client(), 4, &simpleExecutor{taskSvc: deps.taskService})
	go func() {
		workerPool.Start(context.Background())
	}()
	logger.Info("Task queue and worker pool started", zap.Int("workers", 4))
}

func buildRouter(cfg *config.Config) *gin.Engine {
	if cfg.Log.Level != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	return router
}

func registerAllRoutes(router *gin.Engine, deps *serverDependencies, logger *zap.Logger) {
	router.Use(middleware.CORSMiddleware())
	router.Use(middleware.RequestIDMiddleware())
	router.Use(gin.Recovery())
	router.Use(deps.auditLogger.AuditMiddleware())

	// Health check (no auth)
	router.GET("/health", healthCheck)

	// IM Webhook (no auth)
	setupIMWebhook(router)

	// IM per-user bind
	setupIMBind(router, deps.jwtManager, deps.mongoClient)

	// Hermes proxy
	setupHermesProxy(router, logger)

	// System monitoring (no auth)
	router.GET("/api/v1/system/stats", monitor.Handler())

	// Auth routes
	authGroup := router.Group("/api/v1/auth")
	setupAuthRoutes(authGroup, deps.authHandler)

	// Protected API routes
	api := router.Group("/api/v1")
	api.Use(deps.jwtManager.AuthMiddleware())

	setupAuthProtected(api, deps.authHandler)
	setupUserManagement(api, deps.userRepo)
	setupRoleManagement(api, deps.roleRepo)
	setupModelConfig(api, deps.systemConfigRepo, deps.vaultClient)
	setupMemorySearch(api, deps.memoryService)
	setupSysConfig(api, deps.systemConfigRepo)
	setupChangePassword(api, deps.jwtManager, deps.mongoClient)

	// Admin routes
	admin := router.Group("/api/v1/admin")
	admin.Use(deps.jwtManager.AuthMiddleware())
	setupAdminRoutes(admin, deps.authHandler)

	// Chat routes
	chatRoutes := router.Group("/api/v1/chat")
	chatRoutes.Use(deps.jwtManager.AuthMiddleware())
	chatRoutes.POST("", deps.agentService.HandleChat)
	setupChatEnhance(chatRoutes)

	// Agent routes
	setupAgentRoutes(router, deps.jwtManager, deps.agentService)

	// Session routes
	sessionRoutes := router.Group("/api/v1/sessions")
	sessionRoutes.Use(deps.jwtManager.AuthMiddleware())
	setupSessions(sessionRoutes, deps.sessionManager)

	// Artifact routes
	setupArtifactRoutes(router, deps.jwtManager, deps.artifactHandler)

	// Workspace routes
	wsRoutes := router.Group("/api/v1/workspace/:session_id")
	wsRoutes.Use(deps.jwtManager.AuthMiddleware())
	setupWorkspaceRoutes(wsRoutes, deps.artifactHandler)

	// Knowledge Base routes
	setupKnowledgeRoutes(router, deps.jwtManager, deps.kbHandler)

	// Admin KB management
	adminKB := router.Group("/api/v1/admin/knowledge")
	adminKB.Use(deps.jwtManager.AuthMiddleware(), middleware.RequirePermission(model.PermUserManage))
	adminKB.GET("/docs", deps.kbHandler.ListAllDocs)

	// Audit Log routes
	setupAuditRoutes(router, deps.jwtManager, deps.auditHandler)

	// API Review routes
	setupAPIReviewRoutes(router, deps.jwtManager, deps.apiReviewHandler)

	// Notification routes
	setupNotificationRoutes(router, deps.jwtManager, deps.notifHandler)

	// Task routes
	if deps.taskHandler != nil {
		setupTaskRoutes(router, deps.jwtManager, deps.taskHandler)
	}

	// Dashboard routes
	setupDashboard(router, deps.jwtManager, deps.taskService, deps.taskHandler, deps.sessionManager, deps.kbService)
}

func startServer(router *gin.Engine, cfg *config.Config, logger *zap.Logger) {
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

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

// ===================== root-level handlers =====================

func healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func dbUnavailableHandler(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": consts.ErrDBUnavailable})
}

// ===================== IM Webhook =====================

func setupIMWebhook(router *gin.Engine) {
	imService := im.NewService(im.Config{
		AppID:     os.Getenv("FEISHU_APP_ID"),
		AppSecret: os.Getenv("FEISHU_APP_SECRET"),
	})
	router.POST("/api/v1/im/feishu/webhook", func(c *gin.Context) {
		imService.WebhookHandler()(c.Writer, c.Request)
	})
}

// ===================== IM Bind =====================

func setupIMBind(router *gin.Engine, jwtManager *middleware.JWTManager, mongoClient *mongoinfra.Client) {
	imBindGroup := router.Group("/api/v1/im/bind")
	imBindGroup.Use(jwtManager.AuthMiddleware())
	imBindGroup.GET("", getImBindHandler(mongoClient))
	imBindGroup.PUT("", updateImBindHandler(mongoClient))
}

func getImBindHandler(mongoClient *mongoinfra.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		objID, _ := primitive.ObjectIDFromHex(userID.(string))
		var user model.User
		if err := mongoClient.DB().Collection(model.CollUsers).FindOne(c.Request.Context(), bson.M{"_id": objID}).Decode(&user); err != nil {
			c.JSON(http.StatusOK, gin.H{})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"feishu_app_id":     user.FeishuAppID,
			"feishu_app_secret": user.FeishuAppSecret,
		})
	}
}

func updateImBindHandler(mongoClient *mongoinfra.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		objID, _ := primitive.ObjectIDFromHex(userID.(string))
		var req struct {
			FeishuAppID     string `json:"feishu_app_id"`
			FeishuAppSecret string `json:"feishu_app_secret"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.FeishuAppID == "" || req.FeishuAppSecret == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "App ID and App Secret are required"})
			return
		}
		_, err := mongoClient.DB().Collection(model.CollUsers).UpdateOne(c.Request.Context(),
			bson.M{"_id": objID},
			bson.M{"$set": bson.M{"feishu_app_id": req.FeishuAppID, "feishu_app_secret": req.FeishuAppSecret}},
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "\u4fdd\u5b58\u5931\u8d25"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "\u7ed1\u5b9a\u6210\u529f"})
	}
}

// ===================== Hermes Proxy =====================

func setupHermesProxy(router *gin.Engine, logger *zap.Logger) {
	hermesURL := os.Getenv("HERMES_URL")
	if hermesURL != "" {
		router.Any("/api/v1/hermes/*path", hermesProxyHandler(hermesURL))
		logger.Info("Hermes proxy enabled", zap.String("hermes_url", hermesURL))
	}
}

func hermesProxyHandler(hermesURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		target, _ := url.Parse(hermesURL)
		p := httputil.NewSingleHostReverseProxy(target)
		c.Request.URL.Path = c.Param("path")
		p.ServeHTTP(c.Writer, c.Request)
	}
}

// ===================== Auth Routes =====================

func setupAuthRoutes(authGroup *gin.RouterGroup, authHandler *handler.AuthHandler) {
	if authHandler != nil {
		authGroup.POST("/login", authHandler.Login)
		authGroup.POST(consts.PathRegister, authHandler.Register)
		authGroup.GET(consts.PathRegister, authHandler.VerifyInvite)
		authGroup.POST("/complete-registration", authHandler.CompleteRegistration)
	} else {
		authGroup.POST("/login", dbUnavailableHandler)
		authGroup.POST(consts.PathRegister, dbUnavailableHandler)
	}
}

func setupAuthProtected(api *gin.RouterGroup, authHandler *handler.AuthHandler) {
	api.POST("/auth/refresh", refreshTokenHandler(authHandler))
	api.GET("/auth/profile", profileHandler(authHandler))
}

func refreshTokenHandler(authHandler *handler.AuthHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		if authHandler != nil {
			authHandler.RefreshToken(c)
		} else {
			dbUnavailableHandler(c)
		}
	}
}

func profileHandler(authHandler *handler.AuthHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		if authHandler != nil {
			authHandler.GetProfile(c)
		} else {
			dbUnavailableHandler(c)
		}
	}
}

// ===================== User Management =====================

func setupUserManagement(api *gin.RouterGroup, userRepo *mongoinfra.UserRepository) {
	api.GET("/users", middleware.RequirePermission(model.PermUserManage), listUsersHandler(userRepo))
	api.GET(consts.PathUserByID, middleware.RequirePermission(model.PermUserManage), getUserHandler(userRepo))
	api.POST("/users", middleware.RequirePermission(model.PermUserManage), createUserHandler(userRepo))
	api.PUT(consts.PathUserByID, middleware.RequirePermission(model.PermUserManage), updateUserRoleHandler(userRepo))
	api.PATCH("/users/:id/status", middleware.RequirePermission(model.PermUserManage), toggleUserStatusHandler(userRepo))
	api.DELETE(consts.PathUserByID, middleware.RequirePermission(model.PermUserManage), deleteUserHandler(userRepo))
}

func listUsersHandler(userRepo *mongoinfra.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if userRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": consts.ErrDBUnavailable})
			return
		}
		role, _ := c.Get("role")
		skip := int64(0)
		if s := c.Query("skip"); s != "" {
			_, _ = fmt.Sscanf(s, "%d", &skip)
		}
		limit := int64(20)
		if l := c.Query("limit"); l != "" {
			_, _ = fmt.Sscanf(l, "%d", &limit)
		}
		sortBy := c.DefaultQuery("sort_by", "created_at")
		sortOrder := c.DefaultQuery("sort_order", "desc")
		users, total, err := userRepo.ListSorted(c.Request.Context(), role.(string), skip, limit, sortBy, sortOrder)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"users": users, "total": total})
	}
}

func getUserHandler(userRepo *mongoinfra.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if userRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": consts.ErrDBUnavailable})
			return
		}
		user, err := userRepo.FindByID(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if user == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": consts.ErrUserNotFound})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"id":       user.ID.Hex(),
			"username": user.Username,
			"role":     user.Role,
			"status":   user.Status,
		})
	}
}

func createUserHandler(userRepo *mongoinfra.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) { handleCreateUser(c, userRepo) }
}

func handleCreateUser(c *gin.Context, userRepo *mongoinfra.UserRepository) {
	if userRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": consts.ErrDBUnavailable})
		return
	}
	var req struct {
		Username string           `json:"username"`
		Password string           `json:"password"`
		Role     model.UserRole   `json:"role"`
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

	if req.Role == model.RoleSystemAdmin {
		hasAdmin, err := userRepo.HasSystemAdmin(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if hasAdmin {
			c.JSON(http.StatusConflict, gin.H{"error": "\u7cfb\u7edf\u7ba1\u7406\u5458\u5df2\u5b58\u5728\uff0c\u65e0\u6cd5\u521b\u5efa"})
			return
		}
	}

	existing, err := userRepo.FindByUsername(c.Request.Context(), req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "\u8be5\u90ae\u7bb1\u5df2\u88ab\u6ce8\u518c"})
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
}

func updateUserRoleHandler(userRepo *mongoinfra.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if userRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": consts.ErrDBUnavailable})
			return
		}
		userID := c.Param("id")
		user, err := userRepo.FindByID(c.Request.Context(), userID)
		if err != nil || user == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": consts.ErrUserNotFound})
			return
		}
		if user.Role == model.RoleSystemAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "\u4e0d\u80fd\u4fee\u6539\u7cfb\u7edf\u7ba1\u7406\u5458\u7684\u89d2\u8272"})
			return
		}
		var req struct {
			Role model.UserRole `json:"role"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidReq})
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
	}
}

func toggleUserStatusHandler(userRepo *mongoinfra.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if userRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": consts.ErrDBUnavailable})
			return
		}
		userID := c.Param("id")
		user, err := userRepo.FindByID(c.Request.Context(), userID)
		if err != nil || user == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": consts.ErrUserNotFound})
			return
		}
		if user.Role == model.RoleSystemAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "\u4e0d\u80fd\u505c\u7528\u7cfb\u7edf\u7ba1\u7406\u5458"})
			return
		}
		var req struct {
			Status model.UserStatus `json:"status"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidReq})
			return
		}
		if err := userRepo.UpdateStatus(c.Request.Context(), userID, req.Status); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	}
}

func deleteUserHandler(userRepo *mongoinfra.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if userRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": consts.ErrDBUnavailable})
			return
		}
		userID := c.Param("id")
		user, err := userRepo.FindByID(c.Request.Context(), userID)
		if err != nil || user == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": consts.ErrUserNotFound})
			return
		}
		if user.Role == model.RoleSystemAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "\u4e0d\u53ef\u5220\u9664\u7cfb\u7edf\u7ba1\u7406\u5458"})
			return
		}
		if err := userRepo.Delete(c.Request.Context(), userID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	}
}

// ===================== Role Management =====================

func setupRoleManagement(api *gin.RouterGroup, roleRepo *mongoinfra.RoleRepository) {
	api.GET("/roles", middleware.RequirePermission(model.PermUserManage), listRolesHandler(roleRepo))
	api.GET("/permissions", listPermissionsHandler)
	api.POST("/roles", middleware.RequirePermission(model.PermUserManage), createRoleHandler(roleRepo))
	api.PUT("/roles/:id", middleware.RequirePermission(model.PermUserManage), updateRoleHandler(roleRepo))
	api.DELETE("/roles/:id", middleware.RequirePermission(model.PermUserManage), deleteRoleHandler(roleRepo))
}

func listRolesHandler(roleRepo *mongoinfra.RoleRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var customRoles []model.Role
		if roleRepo != nil {
			var err error
			customRoles, err = roleRepo.List(c.Request.Context())
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		fixedRoles := model.FixedRoles()
		allRoles := append(fixedRoles, customRoles...)
		c.JSON(http.StatusOK, gin.H{"roles": allRoles, "total": len(allRoles)})
	}
}

func listPermissionsHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"permissions": model.GetAllPermissions()})
}

func createRoleHandler(roleRepo *mongoinfra.RoleRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if roleRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": consts.ErrDBUnavailable})
			return
		}
		var req struct {
			Name        string   `json:"name"`
			DisplayName string   `json:"display_name"`
			Permissions []string `json:"permissions"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidReq})
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
			"id":           role.ID.Hex(),
			"name":         role.Name,
			"display_name": role.DisplayName,
			"permissions":  role.Permissions,
			"type":         role.Type,
		})
	}
}

func updateRoleHandler(roleRepo *mongoinfra.RoleRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if roleRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": consts.ErrDBUnavailable})
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
			c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidReq})
			return
		}
		if err := roleRepo.Update(c.Request.Context(), roleID, req.Permissions); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	}
}

func deleteRoleHandler(roleRepo *mongoinfra.RoleRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if roleRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": consts.ErrDBUnavailable})
			return
		}
		roleID := c.Param("id")
		role, err := roleRepo.FindByID(c.Request.Context(), roleID)
		if err != nil || role == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
			return
		}
		if role.Type == "fixed" {
			c.JSON(http.StatusForbidden, gin.H{"error": "\u4e0d\u53ef\u5220\u9664\u56fa\u5b9a\u89d2\u8272"})
			return
		}
		if err := roleRepo.Delete(c.Request.Context(), roleID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	}
}

// ===================== Model Config & Vault =====================

func setupModelConfig(api *gin.RouterGroup, systemConfigRepo *mongoinfra.SystemConfigRepository, vaultClient *vaultinfra.Client) {
	api.GET("/model-config", middleware.RequirePermission(model.PermUserManage), getModelConfigHandler(systemConfigRepo))
	api.PUT("/model-config", middleware.RequirePermission(model.PermUserManage), putModelConfigHandler(systemConfigRepo, vaultClient))
	api.POST("/vault/decrypt", middleware.RequirePermission(model.PermUserManage), vaultDecryptHandler(vaultClient))
}

// setupMemorySearch registers the admin memory search endpoint used to verify
// Mem0-style long-term memory writes (SPEC-048/SPEC-046).
func setupMemorySearch(api *gin.RouterGroup, memSvc *adkmemory.Service) {
	api.GET("/memory/search", middleware.RequirePermission(model.PermUserManage), func(c *gin.Context) {
		handleMemorySearch(c, memSvc)
	})
}

// handleMemorySearch searches long-term memory for a user.
// Query params: q (required), user_id (defaults to the caller).
func handleMemorySearch(c *gin.Context, memSvc *adkmemory.Service) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' required"})
		return
	}
	userID := c.Query("user_id")
	if userID == "" {
		uid, _ := c.Get("user_id")
		userID, _ = uid.(string)
	}

	results, err := memSvc.AdminSearch(c.Request.Context(), appName, userID, query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"results": results, "count": len(results)})
}

func getModelConfigHandler(systemConfigRepo *mongoinfra.SystemConfigRepository) gin.HandlerFunc {
	return func(c *gin.Context) { handleGetModelConfig(c, systemConfigRepo) }
}

func handleGetModelConfig(c *gin.Context, systemConfigRepo *mongoinfra.SystemConfigRepository) {
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
				result[cfg.Key] = cfg.Value
				result["api_key_exists"] = true
			} else {
				result[cfg.Key] = cfg.Value
			}
		}
		fillModelConfigDefaults(result)
		result["api_key_exists"] = result["api_key"] != nil && result["api_key"] != ""
		c.JSON(http.StatusOK, result)
		return
	}
	c.JSON(http.StatusOK, configs)
}

func fillModelConfigDefaults(result gin.H) {
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
	// Embedding model defaults (same env-override pattern as LLM config).
	if result["embedding_base_url"] == nil || result["embedding_base_url"] == "" {
		result["embedding_base_url"] = getEnvOrDefault("EMBEDDING_BASE_URL", "http://ollama:11434/v1")
	}
	if result["embedding_model"] == nil || result["embedding_model"] == "" {
		result["embedding_model"] = getEnvOrDefault("EMBEDDING_MODEL", "nomic-embed-text")
	}
	if result["embedding_api_key"] == nil {
		result["embedding_api_key"] = os.Getenv("EMBEDDING_API_KEY")
	}
}

func putModelConfigHandler(systemConfigRepo *mongoinfra.SystemConfigRepository, vaultClient *vaultinfra.Client) gin.HandlerFunc {
	return func(c *gin.Context) { handlePutModelConfig(c, systemConfigRepo, vaultClient) }
}

func handlePutModelConfig(c *gin.Context, systemConfigRepo *mongoinfra.SystemConfigRepository, vaultClient *vaultinfra.Client) {
	if systemConfigRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": consts.ErrDBUnavailable})
		return
	}
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidReq})
		return
	}
	for key, val := range body {
		upsertModelConfigKey(c, systemConfigRepo, vaultClient, key, val)
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func upsertModelConfigKey(c *gin.Context, systemConfigRepo *mongoinfra.SystemConfigRepository, vaultClient *vaultinfra.Client, key string, val interface{}) {
	valStr, ok := val.(string)
	if !ok {
		return
	}
	if (key == "api_key" || key == "hermes_api_key") && valStr != "" {
		if vaultClient != nil {
			vaultPath := vaultinfra.APIKeyPath(consts.DataAgentNS)
			if key == "hermes_api_key" {
				vaultPath = vaultinfra.HermesAPIKeyPath(consts.DataAgentNS)
			}
			if err := vaultClient.Store(c.Request.Context(), vaultPath, valStr); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "vault store failed"})
				return
			}
		}
		_ = systemConfigRepo.Upsert(c.Request.Context(), "model", key, "vault://"+consts.DataAgentNS+"/"+key)
	} else {
		_ = systemConfigRepo.Upsert(c.Request.Context(), "model", key, valStr)
	}
}

func vaultDecryptHandler(vaultClient *vaultinfra.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
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
			c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidReq})
			return
		}
		if req.Key == "" {
			req.Key = vaultinfra.APIKeyPath(consts.DataAgentNS)
		}
		plaintext, err := vaultClient.Retrieve(c.Request.Context(), req.Key)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "vault retrieve failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"plaintext": plaintext, "masked": vaultinfra.MaskValue(plaintext)})
	}
}

// ===================== System Config =====================

func setupSysConfig(api *gin.RouterGroup, systemConfigRepo *mongoinfra.SystemConfigRepository) {
	api.GET("/sysconfig", middleware.RequirePermission(model.PermSystemConfig), getSysConfigHandler(systemConfigRepo))
	api.PUT("/sysconfig", middleware.RequirePermission(model.PermSystemConfig), putSysConfigHandler(systemConfigRepo))
}

func getSysConfigHandler(systemConfigRepo *mongoinfra.SystemConfigRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if systemConfigRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": consts.ErrDBUnavailable})
			return
		}
		dbConfigs, _ := systemConfigRepo.GetAll(c.Request.Context(), "sys")
		result := gin.H{
			"session_recovery_hours": 24,
			"audit_retention_days":   90,
			"notification_ttl_days":  90,
			"email_whitelist":        []string{},
			"report_retry_count":     3,
		}
		for _, cfg := range dbConfigs {
			result[cfg.Key] = cfg.Value
		}
		if s, ok := result["email_whitelist"].(string); ok && s != "" {
			result["email_whitelist"] = strings.Split(s, ",")
		}
		c.JSON(http.StatusOK, result)
	}
}

func putSysConfigHandler(systemConfigRepo *mongoinfra.SystemConfigRepository) gin.HandlerFunc {
	return func(c *gin.Context) { handlePutSysConfig(c, systemConfigRepo) }
}

func handlePutSysConfig(c *gin.Context, systemConfigRepo *mongoinfra.SystemConfigRepository) {
	if systemConfigRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": consts.ErrDBUnavailable})
		return
	}
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidReq})
		return
	}

	if hours, ok := body["session_recovery_hours"]; ok {
		if h, ok := toFloat64(hours); ok && (h < 1 || h > 168) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "\u7f13\u51b2\u671f\u6700\u957f 1 \u5468\uff08168 \u5c0f\u65f6\uff09"})
			return
		}
	}

	for key, val := range body {
		if list, ok := val.([]interface{}); ok {
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
}

// ===================== Admin Routes =====================

func setupAdminRoutes(admin *gin.RouterGroup, authHandler *handler.AuthHandler) {
	admin.GET("/dashboard", middleware.RequirePermission(model.PermSystemConfig), adminDashboardHandler)

	if authHandler != nil {
		admin.POST("/invites", middleware.RequirePermission(model.PermUserManage), authHandler.CreateInvite)
		admin.GET("/invites", middleware.RequirePermission(model.PermUserManage), authHandler.ListInvites)
		admin.DELETE("/invites/:id", middleware.RequirePermission(model.PermUserManage), authHandler.RevokeInvite)
		admin.PUT("/invites/hmac-secret", middleware.RequirePermission(model.PermSystemConfig), authHandler.UpdateHMACSecret)
	}
}

func adminDashboardHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "admin dashboard placeholder"})
}

// ===================== Session Management =====================

func setupSessions(sessionRoutes *gin.RouterGroup, sessionManager *chat.Manager) {
	sessionRoutes.POST("", createSessionHandler(sessionManager))
	sessionRoutes.GET("", listSessionsHandler(sessionManager))
	sessionRoutes.DELETE("/:id", deleteSessionHandler(sessionManager))
	sessionRoutes.POST("/:id/restore", restoreSessionHandler(sessionManager))
	sessionRoutes.GET("/deleted", listDeletedSessionsHandler(sessionManager))
	sessionRoutes.POST("/:id/renew", renewSessionHandler(sessionManager))
}

func createSessionHandler(sessionManager *chat.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		sess, err := sessionManager.Create(userID.(string), "chat")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{
			"session_id": sess.ID,
			"expires_at": sess.ExpiresAt,
		})
	}
}

func listSessionsHandler(sessionManager *chat.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		sessions := sessionManager.ListByUser(userID.(string))
		c.JSON(http.StatusOK, gin.H{"sessions": sessions})
	}
}

func deleteSessionHandler(sessionManager *chat.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := sessionManager.Delete(c.Param("id")); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "deleted"})
	}
}

func restoreSessionHandler(sessionManager *chat.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := sessionManager.Restore(c.Param("id")); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "restored"})
	}
}

func listDeletedSessionsHandler(sessionManager *chat.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		sessions := sessionManager.ListDeleted(userID.(string))
		c.JSON(http.StatusOK, gin.H{"sessions": sessions})
	}
}

func renewSessionHandler(sessionManager *chat.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := sessionManager.Renew(c.Param("id")); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "renewed"})
	}
}

// ===================== Chat Enhance =====================

func setupChatEnhance(chatRoutes *gin.RouterGroup) {
	chatRoutes.POST("/enhance", chatEnhanceHandler)
}

func chatEnhanceHandler(c *gin.Context) {
	var req struct {
		Prompt string `json:"prompt" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "prompt is required"})
		return
	}
	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "default"
	}
	baseURL := os.Getenv("LLM_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	apiKey := os.Getenv("LLM_API_KEY")

	llmReq := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "\u4f60\u662f\u4e00\u4e2a\u63d0\u793a\u8bcd\u4f18\u5316\u4e13\u5bb6\u3002\u628a\u7528\u6237\u8f93\u5165\u7684\u6a21\u7cca\u67e5\u8be2\u8f6c\u5316\u4e3a\u7ed3\u6784\u5316\u3001\u53ef\u64cd\u4f5c\u7684\u6570\u636e\u5206\u6790\u63d0\u793a\u8bcd\uff0c\u5305\u542b\u5177\u4f53\u6307\u6807\u3001\u7ef4\u5ea6\u3001\u65f6\u9650\u548c\u671f\u671b\u8f93\u51fa\u683c\u5f0f\u3002\u76f4\u63a5\u8f93\u51fa\u4f18\u5316\u540e\u7684\u63d0\u793a\u8bcd\uff0c\u4e0d\u8981\u89e3\u91ca\u3002"},
			{"role": "user", "content": req.Prompt},
		},
		"temperature": 0.3,
		"max_tokens":  512,
	}
	body, _ := json.Marshal(llmReq)
	httpReq, err := http.NewRequestWithContext(c.Request.Context(), "POST", baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "\u589e\u5f3a\u670d\u52a1\u4e0d\u53ef\u7528"})
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "\u589e\u5f3a\u670d\u52a1\u4e0d\u53ef\u7528"})
		return
	}
	defer resp.Body.Close()

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.Choices) == 0 {
		c.JSON(http.StatusOK, gin.H{"enhanced": req.Prompt})
		return
	}
	c.JSON(http.StatusOK, gin.H{"enhanced": result.Choices[0].Message.Content})
}

// ===================== Change Password =====================

func setupChangePassword(api *gin.RouterGroup, jwtManager *middleware.JWTManager, mongoClient *mongoinfra.Client) {
	api.POST("/change-password", jwtManager.AuthMiddleware(), changePasswordHandler(mongoClient))
}

func changePasswordHandler(mongoClient *mongoinfra.Client) gin.HandlerFunc {
	return func(c *gin.Context) { handleChangePassword(c, mongoClient) }
}

func handleChangePassword(c *gin.Context, mongoClient *mongoinfra.Client) {
	userID, _ := c.Get("user_id")
	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "\u65e7\u5bc6\u7801\u548c\u65b0\u5bc6\u7801\u4e0d\u80fd\u4e3a\u7a7a"})
		return
	}
	if !validatePasswordComplexity(req.NewPassword) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "\u5bc6\u7801\u81f3\u5c11 8 \u4f4d\uff0c\u9700\u5305\u542b\u5927\u5c0f\u5199\u5b57\u6bcd\u548c\u6570\u5b57"})
		return
	}

	objID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "\u7528\u6237\u4e0d\u5b58\u5728"})
		return
	}
	var user model.User
	coll := mongoClient.DB().Collection(model.CollUsers)
	err = coll.FindOne(c.Request.Context(), bson.M{"_id": objID}).Decode(&user)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "\u7528\u6237\u4e0d\u5b58\u5728"})
		return
	}
	if middleware.CheckPassword(user.PasswordHash, req.OldPassword) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "\u65e7\u5bc6\u7801\u4e0d\u6b63\u786e"})
		return
	}

	newHash, err := middleware.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "\u5bc6\u7801\u52a0\u5bc6\u5931\u8d25"})
		return
	}
	_, err = coll.UpdateOne(c.Request.Context(),
		bson.M{"_id": objID},
		bson.M{"$set": bson.M{"password_hash": newHash, "password_changed": true}},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "\u4fee\u6539\u5931\u8d25"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "\u5bc6\u7801\u4fee\u6539\u6210\u529f"})
}

func validatePasswordComplexity(password string) bool {
	if len(password) < 8 {
		return false
	}
	hasUpper, hasLower, hasDigit := false, false, false
	for _, ch := range password {
		if ch >= 'A' && ch <= 'Z' {
			hasUpper = true
		}
		if ch >= 'a' && ch <= 'z' {
			hasLower = true
		}
		if ch >= '0' && ch <= '9' {
			hasDigit = true
		}
	}
	return hasUpper && hasLower && hasDigit
}

// ===================== Agent Routes =====================

func setupAgentRoutes(router *gin.Engine, jwtManager *middleware.JWTManager, agentService *agent_svc.Service) {
	agentRoutes := router.Group("/api/v1/agent")
	agentRoutes.Use(jwtManager.AuthMiddleware())
	agentRoutes.POST("/tasks", agentService.CreateAgentTask)
	agentRoutes.GET("/tasks/:task_id", agentService.GetAgentTask)
	agentRoutes.GET("/skills", agentService.ListSkills)
	agentRoutes.GET("/skills/search", agentService.SearchSkills)
}

// ===================== Artifact Routes =====================

func setupArtifactRoutes(router *gin.Engine, jwtManager *middleware.JWTManager, artifactHandler *handler.ArtifactHandler) {
	artifactRoutes := router.Group("/api/v1/artifacts")
	artifactRoutes.Use(jwtManager.AuthMiddleware())
	artifactRoutes.POST("/upload", artifactHandler.Upload)
	artifactRoutes.GET("/:id/download", artifactHandler.Download)
	artifactRoutes.DELETE("/:id", artifactHandler.Delete)
	artifactRoutes.GET("", artifactHandler.ListSession)
}

// ===================== Workspace Routes =====================

func setupWorkspaceRoutes(wsRoutes *gin.RouterGroup, artifactHandler *handler.ArtifactHandler) {
	wsRoutes.GET("/files", artifactHandler.ListWorkspace)
	wsRoutes.GET("/files/:filename", artifactHandler.ReadWorkspaceFile)
	wsRoutes.PUT("/files/:filename", artifactHandler.WriteWorkspaceFile)
}

// ===================== Knowledge Routes =====================

func setupKnowledgeRoutes(router *gin.Engine, jwtManager *middleware.JWTManager, kbHandler *handler.KnowledgeHandler) {
	kbRoutes := router.Group("/api/v1/knowledge")
	kbRoutes.Use(jwtManager.AuthMiddleware())
	kbRoutes.POST("/docs", kbHandler.UploadDoc)
	kbRoutes.GET("/docs", kbHandler.ListDocs)
	kbRoutes.GET("/docs/:id", kbHandler.GetDoc)
	kbRoutes.DELETE("/docs/:id", kbHandler.DeleteDoc)
	kbRoutes.POST("/docs/:id/chunks", kbHandler.AddChunks)
	kbRoutes.GET("/search", kbHandler.Search)
}

// ===================== Audit Routes =====================

func setupAuditRoutes(router *gin.Engine, jwtManager *middleware.JWTManager, auditHandler *handler.AuditHandler) {
	auditRoutes := router.Group("/api/v1/admin/audit")
	auditRoutes.Use(jwtManager.AuthMiddleware(), middleware.RequirePermission(model.PermAuditLogView))
	auditRoutes.GET("/logs", auditHandler.ListAuditLogs)
	auditRoutes.POST("/export", auditHandler.ExportAuditLogs)
}

// ===================== API Review Routes =====================

func setupAPIReviewRoutes(router *gin.Engine, jwtManager *middleware.JWTManager, apiReviewHandler *handler.APIReviewHandler) {
	apiRevRoutes := router.Group("/api/v1/admin/api-reviews")
	apiRevRoutes.Use(jwtManager.AuthMiddleware(), middleware.RequirePermission(model.PermAPIConvert))
	apiRevRoutes.GET("", apiReviewHandler.ListAPIReviews)
	apiRevRoutes.POST("", apiReviewHandler.CreateAPIReview)
	apiRevRoutes.PUT("/:id/approve", apiReviewHandler.ApproveAPIReview)
	apiRevRoutes.PUT("/:id/reject", apiReviewHandler.RejectAPIReview)
}

// ===================== Notification Routes =====================

func setupNotificationRoutes(router *gin.Engine, jwtManager *middleware.JWTManager, notifHandler *handler.NotificationHandler) {
	notifRoutes := router.Group("/api/v1/notifications")
	notifRoutes.Use(jwtManager.AuthMiddleware())
	notifRoutes.GET("", notifHandler.ListNotifications)
	notifRoutes.GET("/unread-count", notifHandler.UnreadCount)
	notifRoutes.PUT("/:id/read", notifHandler.MarkRead)
	notifRoutes.PUT("/read-all", notifHandler.MarkAllRead)
	notifRoutes.POST("", notifHandler.SendNotification)
	notifRoutes.POST("/broadcast", notifHandler.BroadcastNotification)
}

// ===================== Task Routes =====================

func setupTaskRoutes(router *gin.Engine, jwtManager *middleware.JWTManager, taskHandler *handler.TaskHandler) {
	taskRoutes := router.Group("/api/v1/tasks")
	taskRoutes.Use(jwtManager.AuthMiddleware())
	taskRoutes.POST("", taskHandler.CreateTask)
	taskRoutes.GET("", taskHandler.ListTasks)
	taskRoutes.GET("/:task_id", taskHandler.GetTask)
	taskRoutes.PUT("/:task_id/cancel", taskHandler.CancelTask)
	taskRoutes.PUT("/:task_id/pause", taskHandler.PauseTask)
	taskRoutes.PUT("/:task_id/resume", taskHandler.ResumeTask)
	taskRoutes.GET("/:task_id/artifacts/download", taskHandler.DownloadArtifacts)

	adminTasks := router.Group("/api/v1/admin/tasks")
	adminTasks.Use(jwtManager.AuthMiddleware(), middleware.RequirePermission(model.PermUserManage))
	adminTasks.GET("", taskHandler.ListAllTasks)
	adminTasks.PUT("/:task_id/retry", taskHandler.RetryTask)
	adminTasks.POST("/batch-cancel", taskHandler.BatchCancelTasks)
}

// ===================== Dashboard Routes =====================

func setupDashboard(router *gin.Engine, jwtManager *middleware.JWTManager, taskService *task_svc.Service, taskHandler *handler.TaskHandler, sessionManager *chat.Manager, kbService *knowledge.Service) {
	router.GET("/api/v1/dashboard", jwtManager.AuthMiddleware(), dashboardHandler(taskService, taskHandler, sessionManager, kbService))
	router.GET("/api/v1/dashboard/trends", jwtManager.AuthMiddleware(), dashboardTrendsHandler(taskService, sessionManager, kbService))
}

func dashboardHandler(taskService *task_svc.Service, taskHandler *handler.TaskHandler, sessionManager *chat.Manager, kbService *knowledge.Service) gin.HandlerFunc {
	return func(c *gin.Context) { handleDashboard(c, taskService, taskHandler, sessionManager, kbService) }
}

func handleDashboard(c *gin.Context, taskService *task_svc.Service, taskHandler *handler.TaskHandler, sessionManager *chat.Manager, kbService *knowledge.Service) {
	stats := monitor.SystemStats()
	userID, _ := c.Get("user_id")

	taskStats := map[string]int{"total": 0, "pending": 0, "running": 0, "completed": 0, "failed": 0}
	sessionCount := 0
	docCount := 0

	if taskHandler != nil && taskService != nil {
		userIDStr := userID.(string)
		tasks, err := taskService.ListTasks(userIDStr)
		if err == nil {
			countTaskStats(tasks, taskStats)
		}
	}

	userSessions := sessionManager.ListByUser(userID.(string))
	sessionCount = len(userSessions)

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
}

func countTaskStats(tasks []task.Task, stats map[string]int) {
	for _, t := range tasks {
		stats["total"]++
		switch string(t.Status) {
		case "pending":
			stats["pending"]++
		case "running":
			stats["running"]++
		case "completed":
			stats["completed"]++
		case "failed":
			stats["failed"]++
		}
	}
}

func dashboardTrendsHandler(taskService *task_svc.Service, sessionManager *chat.Manager, kbService *knowledge.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
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
	}
}

// ===================== Helper Functions =====================

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
		Username:        "\u7cfb\u7edf\u7ba1\u7406\u5458",
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
		zap.String("note", "\u8bf7\u5c3d\u5feb\u4fee\u6539\u5bc6\u7801\uff01\u767b\u5f55\u540e\u6a2a\u5e45\u63d0\u793a\u4fee\u6539"),
	)

	return nil
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

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

type simpleExecutor struct {
	taskSvc *task_svc.Service
}

func (e *simpleExecutor) Execute(ctx context.Context, t *task.Task) error {
	_ = ctx
	_ = t
	return nil
}
