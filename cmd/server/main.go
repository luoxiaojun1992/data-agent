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
	"github.com/luoxiaojun1992/data-agent/internal/adk/memoryx"
	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
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
	"github.com/luoxiaojun1992/data-agent/internal/infra/llmcache"
	"github.com/luoxiaojun1992/data-agent/internal/infra/llmstats"
	mongoinfra "github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
	qdrantinfra "github.com/luoxiaojun1992/data-agent/internal/infra/qdrant"
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

	"google.golang.org/adk/memory"
	adkmodel "google.golang.org/adk/model"
	genai "google.golang.org/genai"
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
	modelCfg         *modelcfg.Provider
	qdrantClient     *qdrantinfra.Client
	adkRuntime       *adkruntime.Runtime
	adkSessions      *adksession.Service
	memoryService    memory.Service // google.golang.org/adk/memory.Service
	memoryKit        *memoryx.Kit   // adk-go-memory Kit (nil when legacy)
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
	llmRecorder      *llmstats.Recorder
	llmCache         *llmcache.Cache
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
	deps.qdrantClient = qdrantinfra.NewClient(getEnvOrDefault("QDRANT_URL", "qdrant:6334"))

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
	deps.modelCfg = modelcfg.NewProvider(deps.systemConfigRepo)
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

func initMemoryBackend(deps *serverDependencies, mongoClient *mongoinfra.Client, llm adkmodel.LLM, logger *zap.Logger) {
	embedFn := buildEmbedFn(deps)
	// Wrap embedFn with Redis cache + token recording.
	if deps.llmCache != nil || deps.llmRecorder != nil {
		embedFn = cachedEmbedFn(embedFn, deps.llmCache, deps.llmRecorder,
			getEnvOrDefault("EMBEDDING_MODEL", "embedding"))
	}
	if os.Getenv("MEMORY_BACKEND") == "legacy" {
		// Legacy path removed per SPEC-053: only adk-go-memory (memoryx) supported.
		logger.Warn("MEMORY_BACKEND=legacy is deprecated, using adk-go-memory")
	}
	logger.Info("Using adk-go-memory backend (SPEC-050)")
	kit, err := memoryx.NewKit(mongoClient.DB(), appName, llm, embedFn)
	if err != nil {
		logger.Fatal("Failed to create adk-go-memory Kit", zap.Error(err))
	}
	deps.memoryService = kit.Service()
	deps.memoryKit = kit
}

func buildEmbedFn(deps *serverDependencies) func(ctx context.Context, text string) ([]float32, error) {
	cfg := deps.modelCfg.EmbeddingConfig()
	if cfg.BaseURL == "" {
		return nil
	}
	e := adkmemory.NewOpenAIEmbedding(adkmemory.OpenAIEmbeddingConfig{
		BaseURL: cfg.BaseURL, Model: cfg.Model, APIKey: cfg.APIKey,
	})
	return func(ctx context.Context, text string) ([]float32, error) { return e(ctx, text) }
}

func initServices(deps *serverDependencies, mongoClient *mongoinfra.Client, logger *zap.Logger) {
	deps.sessionManager = chat.NewManager(mongoinfra.NewSessionRepository(mongoClient.DB()), 24*time.Hour)
	deps.llmRecorder = llmstats.NewRecorder(mongoClient.DB())
	if deps.redisClient != nil {
		deps.llmCache = llmcache.New(deps.redisClient.Client())
	}

	// Build LLM from model config (Provider reads system_config or env).
	llm, llmErr := deps.modelCfg.BuildLLM(context.Background(), "")
	if llmErr != nil {
		logger.Fatal("Failed to build LLM from model config", zap.Error(llmErr))
	}

	// Compaction LLM — separate config for session summarization.
	compactionLLM, cErr := deps.modelCfg.BuildLLM(context.Background(), modelcfg.UseCaseCompaction)
	if cErr != nil {
		compactionLLM = llm // fallback to chat LLM
	}

	// ADK session service (MongoDB) with LLM-summarization compaction.
	deps.adkSessions = adksession.NewService(mongoClient.DB()).WithCompaction(
		adksession.CompactionConfig{MaxEvents: 100, MaxTokens: 4000, KeepRecent: 20},
		adksession.NewLLMSummarizer(compactionLLM),
	)

	// Long-term memory (MongoDB + embedding).
	initMemoryBackend(deps, mongoClient, compactionLLM, logger)

	// ADK tools.
	toolDeps := &adktools.Deps{
		KBService:    deps.kbService,
		Memory:       deps.memoryService,
		MemoryWriter: deps.memoryKit,
		AppName:      appName,
	}
	tools, err := adktools.All(toolDeps)
	if err != nil {
		logger.Fatal("Failed to build ADK tools", zap.Error(err))
	}

	// ADK runtime.
	rt, err := adkruntime.New(adkruntime.Config{
		AppName:        appName,
		Model:          llm,
		SessionService: deps.adkSessions,
		MemoryService:  deps.memoryService,
		Tools:          tools,
		Auditor:        deps.secAuditor,
		Instruction:    deps.modelCfg.DefaultInstruction(context.Background()),
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
	artifactRepo := mongoinfra.NewArtifactRepository(mongoClient.DB())
	fileStore := seaweedfs.NewFileStore(deps.swClient)
	deps.artifactStorage = artifact_svc.NewStorage(fileStore, artifactRepo)
	deps.workspaceMgr = workspace.NewManager(deps.artifactStorage)
	deps.artifactHandler = handler.NewArtifactHandler(deps.artifactStorage, deps.workspaceMgr)
}

func initKnowledgeBase(deps *serverDependencies, mongoClient *mongoinfra.Client) {
	deps.kbService = knowledge.NewService(mongoinfra.NewKBRepository(mongoClient.DB()))
	embCfg := deps.modelCfg.EmbeddingConfig()
	if embCfg.BaseURL != "" && deps.qdrantClient != nil {
		embedFn := adkmemory.NewOpenAIEmbedding(adkmemory.OpenAIEmbeddingConfig{
			BaseURL: embCfg.BaseURL, Model: embCfg.Model, APIKey: embCfg.APIKey,
		})
		rawEmbed := func(ctx context.Context, text string) ([]float32, error) { return embedFn(ctx, text) }
		kEmbedFn := cachedEmbedFn(rawEmbed, deps.llmCache, deps.llmRecorder, embCfg.Model)
		vectorStore := qdrantinfra.NewVectorStore(deps.qdrantClient)
		deps.kbService.WithVectorIndex(vectorStore, knowledge.EmbeddingFunc(kEmbedFn))
	}
	deps.kbHandler = handler.NewKnowledgeHandler(deps.kbService)
}

func initAuditAndNotifications(deps *serverDependencies, mongoClient *mongoinfra.Client) {
	deps.auditService = auditsvc.NewService(mongoinfra.NewAuditRepository(mongoClient.DB()))
	deps.auditHandler = handler.NewAuditHandler(deps.auditService)
	deps.apiReviewSvc = apireview.NewService(mongoinfra.NewAPIReviewRepository(mongoClient.DB()))
	deps.apiReviewHandler = handler.NewAPIReviewHandler(deps.apiReviewSvc)
	deps.notifSvc = notifsvc.NewService(mongoinfra.NewNotificationRepository(mongoClient.DB()))
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

	deps.taskService = task_svc.NewService(mongoinfra.NewTaskRepository(mongoClient.DB()), taskStream)
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
	handler.RegisterUserRoutes(api, handler.NewUserHandler(deps.userRepo))
	handler.RegisterRoleRoutes(api, handler.NewRoleHandler(deps.roleRepo))
	handler.RegisterModelConfigRoutes(api, handler.NewModelConfigHandler(deps.systemConfigRepo))
	setupMemorySearch(api, deps.memoryService)
	sysCfgHandler := handler.NewConfigHandler(deps.systemConfigRepo, deps.roleRepo)
	handler.RegisterSysConfigRoutes(api, sysCfgHandler)

	// Admin routes
	admin := router.Group("/api/v1/admin")
	admin.Use(deps.jwtManager.AuthMiddleware())
	setupAdminRoutes(admin, deps.authHandler)

	// Chat routes
	chatRoutes := router.Group("/api/v1/chat")
	chatRoutes.Use(deps.jwtManager.AuthMiddleware())
	chatRoutes.POST("", deps.agentService.HandleChat)
	setupChatEnhance(chatRoutes, deps)

	// Agent routes
	setupAgentRoutes(router, deps.jwtManager, deps.agentService)

	// Session routes
	sessionRoutes := router.Group("/api/v1/sessions")
	sessionRoutes.Use(deps.jwtManager.AuthMiddleware())
	handler.RegisterSessionRoutes(sessionRoutes, handler.NewSessionHandler(deps.sessionManager))

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
	handler.RegisterDashboardRoutes(router, deps.jwtManager.AuthMiddleware(), handler.NewDashboardHandler(deps.taskService, deps.taskHandler, deps.sessionManager, deps.kbService))
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


// defaultModel is the fallback model name for enhance/embedding.
const defaultModel = "gpt-4o"

// cachedEmbedFn wraps an embedding function with Redis cache and token recording.
func cachedEmbedFn(raw adkmemory.EmbeddingFunc, cache *llmcache.Cache, rec *llmstats.Recorder, model string) adkmemory.EmbeddingFunc {
	if cache == nil && rec == nil {
		return raw
	}
	return func(ctx context.Context, text string) ([]float32, error) {
		vec, cacheHit := lookupEmbeddingCache(ctx, cache, model, text)
		if !cacheHit {
			var err error
			vec, err = raw(ctx, text)
			if err != nil {
				return nil, err
			}
		}
		recordEmbeddingCall(ctx, rec, model, text, cacheHit)
		if !cacheHit {
			storeEmbeddingCache(ctx, cache, model, text, vec)
		}
		return vec, nil
	}
}

func lookupEmbeddingCache(ctx context.Context, cache *llmcache.Cache, model, text string) ([]float32, bool) {
	if cache == nil {
		return nil, false
	}
	cached, ok := cache.GetEmbedding(ctx, model, text)
	if !ok {
		return nil, false
	}
	return adkmemory.ParseCachedEmbedding(cached), true
}

func recordEmbeddingCall(ctx context.Context, rec *llmstats.Recorder, model, text string, cacheHit bool) {
	if rec == nil {
		return
	}
	_ = rec.Record(ctx, llmstats.Record{
		CallPoint:    "embedding",
		Model:        model,
		PromptTokens: llmstats.EstimateTokens(text),
		Estimated:    true,
		CacheHit:     cacheHit,
	})
}

func storeEmbeddingCache(ctx context.Context, cache *llmcache.Cache, model, text string, vec []float32) {
	if cache == nil {
		return
	}
	cache.SetEmbedding(ctx, model, text, adkmemory.MarshalCachedEmbedding(vec))
}

// callEnhanceLLM calls the LLM to enhance a prompt. Falls back to original on error.
func callEnhanceLLM(ctx context.Context, prompt string) string {
	model := getEnvOrDefault("LLM_MODEL", "gpt-4o")
	baseURL := getEnvOrDefault("LLM_BASE_URL", "https://api.openai.com")
	apiKey := os.Getenv("LLM_API_KEY")

	llmReq := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是一个提示词优化专家。把用户输入的模糊查询转化为结构化、可操作的数据分析提示词，包含具体指标、维度、时限和期望输出格式。直接输出优化后的提示词，不要解释。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
		"max_tokens":  512,
	}
	body, _ := json.Marshal(llmReq)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(httpReq)
	if err != nil {
		return prompt
	}
	defer resp.Body.Close()

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Choices) == 0 {
		return prompt
	}
	return result.Choices[0].Message.Content
}

// recordEnhanceTokens records token usage for an enhance call.
func recordEnhanceTokens(ctx context.Context, deps *serverDependencies, prompt, enhanced string) {
	if deps.llmRecorder == nil {
		return
	}
	model := getEnvOrDefault("LLM_MODEL", defaultModel)
	_ = deps.llmRecorder.Record(ctx, llmstats.Record{
		CallPoint:        "enhance",
		Model:            model,
		PromptTokens:     llmstats.EstimateTokens(prompt),
		CompletionTokens: llmstats.EstimateTokens(enhanced),
		Estimated:        true,
	})
}

func makeEnhanceHandler(deps *serverDependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Prompt string `json:"prompt"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Prompt == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidReq})
			return
		}

		ctx := c.Request.Context()
		prompt := req.Prompt
		modelName := getEnvOrDefault("LLM_MODEL", "default")

		// Redis cache lookup.
		if deps.llmCache != nil {
			if cached, ok := deps.llmCache.GetEnhance(ctx, modelName, prompt); ok {
				c.JSON(http.StatusOK, gin.H{"enhanced": cached})
				return
			}
		}

		enhanced := enhanceViaADK(ctx, deps, prompt)
		if deps.llmCache != nil {
			deps.llmCache.SetEnhance(ctx, modelName, prompt, enhanced)
		}
		recordEnhanceTokens(ctx, deps, prompt, enhanced)
		c.JSON(http.StatusOK, gin.H{"enhanced": enhanced})
	}
}

// enhanceViaADK uses the ADK model router for prompt enhancement, falling back to
// direct HTTP on error.
func enhanceViaADK(ctx context.Context, deps *serverDependencies, prompt string) string {
	llm, lErr := deps.modelCfg.BuildLLM(ctx, modelcfg.UseCaseEnhance)
	if lErr != nil {
		return callEnhanceLLM(ctx, prompt)
	}
	sys := "你是提示词优化专家。把用户输入的模糊查询转化为结构化、可操作的数据分析提示词，包含具体指标、维度、时限和期望输出格式。直接输出优化后的提示词，不要解释。"
	temp := float32(0.3)
	adkReq := &adkmodel.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{genai.NewPartFromText(sys)}},
			{Role: "user", Parts: []*genai.Part{genai.NewPartFromText(prompt)}},
		},
		Config: &genai.GenerateContentConfig{MaxOutputTokens: 512, Temperature: &temp},
	}
	for resp, err := range llm.GenerateContent(ctx, adkReq, false) {
		if err != nil {
			return callEnhanceLLM(ctx, prompt)
		}
		if resp.Content != nil && len(resp.Content.Parts) > 0 {
			return resp.Content.Parts[0].Text
		}
	}
	return callEnhanceLLM(ctx, prompt)
}

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

func dashboardHandler(taskService *task_svc.Service, taskHandler *handler.TaskHandler, sessionManager *chat.Manager, kbService *knowledge.Service) gin.HandlerFunc {
	return func(c *gin.Context) { handleDashboard(c, taskService, taskHandler, sessionManager, kbService) }
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

// doEnhanceCall calls the LLM to enhance a prompt and returns the result with token usage.

// === Restored setup functions (migrated handlers delegate here) ===

func setupIMBind(router *gin.Engine, jwtManager *middleware.JWTManager, mongoClient *mongoinfra.Client) {
	imBindGroup := router.Group("/api/v1/im/bind")
	imBindGroup.Use(jwtManager.AuthMiddleware())
	imBindGroup.GET("", getImBindHandler(mongoClient))
	imBindGroup.PUT("", updateImBindHandler(mongoClient))
}

func setupHermesProxy(router *gin.Engine, logger *zap.Logger) {
	hermesURL := os.Getenv("HERMES_URL")
	if hermesURL != "" {
		router.Any("/api/v1/hermes/*path", hermesProxyHandler(hermesURL))
		logger.Info("Hermes proxy enabled", zap.String("hermes_url", hermesURL))
	}
}

func setupMemorySearch(api *gin.RouterGroup, memSvc memory.Service) {
	api.GET("/memory/search", middleware.RequirePermission(model.PermUserManage), func(c *gin.Context) {
		handleMemorySearch(c, memSvc)
	})
}

func handleMemorySearch(c *gin.Context, memSvc memory.Service) {
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

	results, err := memSvc.SearchMemory(c.Request.Context(), &memory.SearchRequest{
		Query:   query,
		UserID:  userID,
		AppName: appName,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var texts []string
	for _, m := range results.Memories {
		if m.Content != nil {
			for _, p := range m.Content.Parts {
				if p != nil {
					texts = append(texts, p.Text)
				}
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"results": texts, "count": len(texts)})
}

func setupAdminRoutes(admin *gin.RouterGroup, authHandler *handler.AuthHandler) {
	admin.GET("/dashboard", middleware.RequirePermission(model.PermSystemConfig), adminDashboardHandler)

	if authHandler != nil {
		admin.POST("/invites", middleware.RequirePermission(model.PermUserManage), authHandler.CreateInvite)
		admin.GET("/invites", middleware.RequirePermission(model.PermUserManage), authHandler.ListInvites)
		admin.DELETE("/invites/:id", middleware.RequirePermission(model.PermUserManage), authHandler.RevokeInvite)
		admin.PUT("/invites/hmac-secret", middleware.RequirePermission(model.PermSystemConfig), authHandler.UpdateHMACSecret)
	}
}

func setupChatEnhance(chatRoutes *gin.RouterGroup, deps *serverDependencies) {
	chatRoutes.POST("/enhance", makeEnhanceHandler(deps))
}

func makeEnhanceHandler(deps *serverDependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Prompt string `json:"prompt"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Prompt == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidReq})
			return
		}

		ctx := c.Request.Context()
		prompt := req.Prompt
		modelName := getEnvOrDefault("LLM_MODEL", "default")

		// Redis cache lookup.
		if deps.llmCache != nil {
			if cached, ok := deps.llmCache.GetEnhance(ctx, modelName, prompt); ok {
				c.JSON(http.StatusOK, gin.H{"enhanced": cached})
				return
			}
		}

		enhanced := enhanceViaADK(ctx, deps, prompt)
		if deps.llmCache != nil {
			deps.llmCache.SetEnhance(ctx, modelName, prompt, enhanced)
		}
		recordEnhanceTokens(ctx, deps, prompt, enhanced)
		c.JSON(http.StatusOK, gin.H{"enhanced": enhanced})
	}
}

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
