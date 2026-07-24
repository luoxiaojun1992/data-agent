// Package main — wiring helpers. This file holds the construction of every
// service, handler, and infra client. main.go itself only orchestrates
// startup/shutdown; the heavy lifting lives here so the entry point stays
// readable (SPEC-058: main.go ≤ 300 lines).
package main

import (
	"context"
	"net/http"
	"os"
	"sync"
	"time"

	adkmemory "github.com/luoxiaojun1992/data-agent/internal/adk/memory"
	"github.com/luoxiaojun1992/data-agent/internal/adk/memoryx"
	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
	adkruntime "github.com/luoxiaojun1992/data-agent/internal/adk/runtime"
	adksession "github.com/luoxiaojun1992/data-agent/internal/adk/session"
	adktools "github.com/luoxiaojun1992/data-agent/internal/adk/tools"
	"github.com/luoxiaojun1992/data-agent/internal/api/handler"
	"github.com/luoxiaojun1992/data-agent/internal/config"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
	"github.com/luoxiaojun1992/data-agent/internal/infra/llmcache"
	"github.com/luoxiaojun1992/data-agent/internal/infra/llmstats"
	mongoinfra "github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
	qdrantinfra "github.com/luoxiaojun1992/data-agent/internal/infra/qdrant"
	"github.com/luoxiaojun1992/data-agent/internal/infra/redis"
	"github.com/luoxiaojun1992/data-agent/internal/infra/seaweedfs"
	vaultinfra "github.com/luoxiaojun1992/data-agent/internal/infra/vault"
	"github.com/luoxiaojun1992/data-agent/internal/logic"
	agentlogic "github.com/luoxiaojun1992/data-agent/internal/logic/agent"
	"github.com/luoxiaojun1992/data-agent/internal/logic/workspace"
	"github.com/luoxiaojun1992/data-agent/internal/queue"
	"github.com/luoxiaojun1992/data-agent/internal/scheduler"
	apireview "github.com/luoxiaojun1992/data-agent/internal/service/apireview"
	artifact_svc "github.com/luoxiaojun1992/data-agent/internal/service/artifact"
	auditsvc "github.com/luoxiaojun1992/data-agent/internal/service/audit"
	authsvc "github.com/luoxiaojun1992/data-agent/internal/service/auth"
	"github.com/luoxiaojun1992/data-agent/internal/service/chat"
	configsvc "github.com/luoxiaojun1992/data-agent/internal/service/config"
	enhancesvc "github.com/luoxiaojun1992/data-agent/internal/service/enhance"
	"github.com/luoxiaojun1992/data-agent/internal/service/im"
	"github.com/luoxiaojun1992/data-agent/internal/service/knowledge"
	notifsvc "github.com/luoxiaojun1992/data-agent/internal/service/notification"
	"github.com/luoxiaojun1992/data-agent/internal/service/role"
	task_svc "github.com/luoxiaojun1992/data-agent/internal/service/task"
	"github.com/luoxiaojun1992/data-agent/internal/service/user"
	"github.com/luoxiaojun1992/data-agent/internal/worker"
	"go.uber.org/zap"

	adkmodel "google.golang.org/adk/model"
	adksessionIF "google.golang.org/adk/session"
)

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

func initADKModel(deps *serverDependencies) {
	deps.modelCfg = modelcfg.NewProvider(deps.sysConfigCacheRepo)
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
	if deps.llmCache != nil || deps.llmRecorder != nil {
		embedFn = cachedEmbedFn(embedFn, deps.llmCache, deps.llmRecorder,
			getEnvOrDefault("EMBEDDING_MODEL", "embedding"))
	}
	if os.Getenv("MEMORY_BACKEND") == "legacy" {
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

// buildEmbedFn creates an embedding function that reads the embedding config
// on every call (via cache→DB) instead of baking a snapshot at startup.
// This ensures config changes (model/baseURL/APIKey) take effect without
// restart (SPEC-061 §5.3.1 — eliminate config value preloading).
//
// The embedder instance is cached and reused as long as the config is
// unchanged; it is rebuilt when the config differs (instance cache, not
// config cache — does not violate rule 5). A mutex guards the instance
// swap; the actual embedding call runs outside the lock to avoid
// serialising concurrent requests.
func buildEmbedFn(deps *serverDependencies) func(ctx context.Context, text string) ([]float32, error) {
	var (
		mu       sync.Mutex
		lastCfg  modelcfg.EmbeddingEntry
		embedder func(ctx context.Context, text string) ([]float32, error)
	)
	return func(ctx context.Context, text string) ([]float32, error) {
		cfg := deps.modelCfg.EmbeddingConfig() // reads cache→DB each call, hot-reload
		if cfg.BaseURL == "" {
			return nil, nil
		}
		mu.Lock()
		if embedder == nil || cfg != lastCfg {
			e := adkmemory.NewOpenAIEmbedding(adkmemory.OpenAIEmbeddingConfig{
				BaseURL: cfg.BaseURL, Model: cfg.Model, APIKey: cfg.APIKey,
			})
			embedder = func(ctx context.Context, text string) ([]float32, error) { return e(ctx, text) }
			lastCfg = cfg
		}
		fn := embedder
		mu.Unlock()
		return fn(ctx, text)
	}
}

func initServices(deps *serverDependencies, mongoClient *mongoinfra.Client, logger *zap.Logger) {
	deps.sessionManager = chat.NewManager(mongoinfra.NewSessionRepository(mongoClient.DB()), 24*time.Hour)
	deps.llmRecorder = llmstats.NewRecorder(mongoClient.DB())
	if deps.redisClient != nil {
		deps.llmCache = llmcache.New(deps.redisClient.Client())
	}

	// SPEC-062: System-level compaction LLM (baked into the shared ADK
	// SessionService). This is a single system LLM, not the per-session model
	// — per-session Runtimes are lazily created by the Registry. compactionLLM
	// reads config via the SPEC-061 cache so it picks up config on restart.
	llm, llmErr := deps.modelCfg.BuildLLM(context.Background(), "")
	if llmErr != nil {
		logger.Fatal("Failed to build compaction LLM from model config", zap.Error(llmErr))
	}
	compactionLLM := llm
	if cl, cErr := deps.modelCfg.BuildLLM(context.Background(), modelcfg.UseCaseCompaction); cErr == nil {
		compactionLLM = cl
	}

	deps.adkSessions = adksession.NewService(mongoClient.DB()).WithCompaction(
		adksession.CompactionConfig{MaxEvents: 100, MaxTokens: 4000, KeepRecent: 20},
		adksession.NewLLMSummarizer(compactionLLM),
	)

	initMemoryBackend(deps, mongoClient, compactionLLM, logger)

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

	// SPEC-062: Build the per-model Runtime registry (lazy create + fingerprint
	// hot-reload). Replaces the single shared Runtime; chat.Service resolves a
	// Runtime per session.ModelID at run time.
	deps.registry = adkruntime.NewRegistry(adkruntime.RegistryConfig{
		Provider:       deps.modelCfg,
		SessionService: deps.adkSessions,
		MemoryService:  deps.memoryService,
		Tools:          tools,
		Auditor:        deps.secAuditor,
		AppName:        appName,
	})

	deps.chatService = chat.NewService(deps.registry, deps.modelCfg, deps.adkSessions, deps.sessionManager, deps.cbRegistry).
		WithMemoryWrite(func(ctx context.Context, sess adksessionIF.Session) {
			if err := deps.memoryService.AddSessionToMemory(ctx, sess); err != nil {
				logger.Warn("memory write failed", zap.Error(err))
			}
		})

	// Orchestrator coordinates session + task for async agent tasks (SPEC-058,
	// SPEC-062: provider resolves default model for task binding).
	deps.orchestrator = agentlogic.NewOrchestrator(deps.sessionManager, deps.taskService, deps.modelCfg)
}

func initEnhance(deps *serverDependencies) {
	deps.enhanceService = enhancesvc.NewService(deps.modelCfg, deps.llmCache, deps.llmRecorder)
}

func initIM(deps *serverDependencies) {
	deps.imService = im.NewService(im.Config{
		AppID:     os.Getenv("FEISHU_APP_ID"),
		AppSecret: os.Getenv("FEISHU_APP_SECRET"),
	})
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
	// SPEC-061: Use on-demand embed function (reads cache→DB per call) instead
	// of preloading embedding config at startup. Vector index is set up whenever
	// Qdrant is available; the embed function returns (nil, nil) if config is
	// empty, and picks up new config without restart once admin configures it.
	if deps.qdrantClient != nil {
		rawEmbed := buildEmbedFn(deps)
		kEmbedFn := cachedEmbedFn(rawEmbed, deps.llmCache, deps.llmRecorder,
			getEnvOrDefault("EMBEDDING_MODEL", "embedding"))
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

	// SPEC-061: Inject Redis-backed cache into the SysConfig Cache-Aside
	// decorator. Until this point the decorator degrades to direct mongo
	// reads (cache==nil). After injection, all config reads go through
	// Redis first (cache→DB on miss), enabling hot-reload of config values.
	if deps.sysConfigCacheRepo != nil {
		deps.sysConfigCacheRepo.SetCache(redis.NewCacheRepo(redisClient.Client()))
		logger.Info("SysConfig cache decorator activated (Redis)")
	}

	taskStream, streamErr := queue.NewStream(redisClient.Client())
	if streamErr != nil {
		logger.Warn("Failed to create task stream", zap.Error(streamErr))
		return
	}
	deps.taskStream = taskStream

	deps.taskService = task_svc.NewService(mongoinfra.NewTaskRepository(mongoClient.DB()), queue.QueueRepository(taskStream))
	deps.taskHandler = handler.NewTaskHandler(deps.taskService)
	// Re-wire the orchestrator now that the task service exists.
	if deps.orchestrator != nil {
		deps.orchestrator = agentlogic.NewOrchestrator(deps.sessionManager, deps.taskService, deps.modelCfg)
	}

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

// buildRouteDeps constructs the handler wiring for route registration. All
// HTTP handlers are built here; main.go itself defines no handler funcs.
func buildRouteDeps(deps *serverDependencies, cfg *config.Config, logger *zap.Logger) *handler.RouteDeps {
	cfgSvc := configsvc.NewService(deps.sysConfigCacheRepo)
	roleSvc := role.NewService(deps.roleRepo)

	var imWebhook http.HandlerFunc
	if deps.imService != nil {
		imWebhook = deps.imService.WebhookHandler()
	}

	var imBindHandler *handler.IMBindHandler
	if deps.mongoClient != nil {
		imBindHandler = handler.NewIMBindHandler(im.NewBindService(mongoinfra.NewIMBindRepository(deps.mongoClient.DB())))
	}

	toolLister := handler.ToolListerFunc(func() []string {
		names, err := adktools.Names(&adktools.Deps{
			KBService:    deps.kbService,
			Memory:       deps.memoryService,
			MemoryWriter: deps.memoryKit,
			AppName:      appName,
		})
		if err != nil {
			return []string{}
		}
		return names
	})

	return &handler.RouteDeps{
		JWTManager:    deps.jwtManager,
		AuditLogger:   deps.auditLogger,
		Auth:          deps.authHandler,
		User:          handler.NewUserHandler(user.NewService(deps.userRepo, user.NewBcryptHasher())),
		Role:          handler.NewRoleHandler(roleSvc),
		ModelConfig:   handler.NewModelConfigHandler(cfgSvc, deps.modelCfg),
		SysConfig:     handler.NewConfigHandler(cfgSvc, roleSvc, deps.userRepo),
		Memory:        handler.NewMemoryHandler(deps.memoryService, appName),
		Chat:          handler.NewChatHandler(deps.chatService),
		Enhance:       handler.NewEnhanceHandler(deps.enhanceService),
		Agent:         handler.NewAgentHandler(deps.orchestrator, deps.taskService, toolLister),
		Session:       handler.NewSessionHandler(deps.sessionManager),
		Artifact:      deps.artifactHandler,
		Knowledge:     deps.kbHandler,
		Audit:         deps.auditHandler,
		APIReview:     deps.apiReviewHandler,
		Notification:  deps.notifHandler,
		Task:          deps.taskHandler,
		Dashboard:     handler.NewDashboardHandler(deps.taskService, deps.kbService, deps.llmRecorder),
		IMBind:        imBindHandler,
		Stats:         handler.NewStatsHandler(deps.llmRecorder),
		IMWebhook:     imWebhook,
		HermesURL:     os.Getenv("HERMES_URL"),
		AppName:       appName,
		MemoryService: deps.memoryService,
	}
}

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
