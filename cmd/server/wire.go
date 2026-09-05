// Package main — wiring helpers. This file holds the construction of every
// service, handler, and infra client. main.go itself only orchestrates
// startup/shutdown; the heavy lifting lives here so the entry point stays
// readable (SPEC-058: main.go ≤ 300 lines).
package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	adkmemory "github.com/luoxiaojun1992/data-agent/internal/adk/memory"
	"github.com/luoxiaojun1992/data-agent/internal/adk/memoryx"
	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
	adkruntime "github.com/luoxiaojun1992/data-agent/internal/adk/runtime"
	adksession "github.com/luoxiaojun1992/data-agent/internal/adk/session"
	"github.com/luoxiaojun1992/data-agent/internal/adk/subagent"
	adktools "github.com/luoxiaojun1992/data-agent/internal/adk/tools"
	"github.com/luoxiaojun1992/data-agent/internal/api/handler"
	"github.com/luoxiaojun1992/data-agent/internal/config"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
	arcadedbinfra "github.com/luoxiaojun1992/data-agent/internal/infra/arcadedb"
	"github.com/luoxiaojun1992/data-agent/internal/infra/cache"
	"github.com/luoxiaojun1992/data-agent/internal/infra/llmcache"
	"github.com/luoxiaojun1992/data-agent/internal/infra/llmstats"
	"github.com/luoxiaojun1992/data-agent/internal/infra/metrics"
	"github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
	mongoinfra "github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
	mysqlinfra "github.com/luoxiaojun1992/data-agent/internal/infra/mysql"
	qdrantinfra "github.com/luoxiaojun1992/data-agent/internal/infra/qdrant"
	"github.com/luoxiaojun1992/data-agent/internal/infra/redis"
	"github.com/luoxiaojun1992/data-agent/internal/infra/seaweedfs"
	vaultinfra "github.com/luoxiaojun1992/data-agent/internal/infra/vault"
	"github.com/luoxiaojun1992/data-agent/internal/logic"
	agentlogic "github.com/luoxiaojun1992/data-agent/internal/logic/agent"
	"github.com/luoxiaojun1992/data-agent/internal/logic/workspace"
	"github.com/luoxiaojun1992/data-agent/internal/queue"
	"github.com/luoxiaojun1992/data-agent/internal/scheduler"
	apicollectionsvc "github.com/luoxiaojun1992/data-agent/internal/service/apicollection"
	artifact_svc "github.com/luoxiaojun1992/data-agent/internal/service/artifact"
	auditsvc "github.com/luoxiaojun1992/data-agent/internal/service/audit"
	authsvc "github.com/luoxiaojun1992/data-agent/internal/service/auth"
	"github.com/luoxiaojun1992/data-agent/internal/service/chat"
	configsvc "github.com/luoxiaojun1992/data-agent/internal/service/config"
	enhancesvc "github.com/luoxiaojun1992/data-agent/internal/service/enhance"
	feishu_svc "github.com/luoxiaojun1992/data-agent/internal/service/feishu"
	"github.com/luoxiaojun1992/data-agent/internal/service/guard"
	"github.com/luoxiaojun1992/data-agent/internal/service/humanchannel"
	"github.com/luoxiaojun1992/data-agent/internal/service/im"
	"github.com/luoxiaojun1992/data-agent/internal/service/knowledge"
	"github.com/luoxiaojun1992/data-agent/internal/service/monitor"
	notifsvc "github.com/luoxiaojun1992/data-agent/internal/service/notification"
	"github.com/luoxiaojun1992/data-agent/internal/service/pii"
	rbacsvc "github.com/luoxiaojun1992/data-agent/internal/service/rbac"
	skillsvc "github.com/luoxiaojun1992/data-agent/internal/service/skill"
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
	authService.SetSysConfigCache(deps.sysConfigCacheRepo)
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

func initADKModel(deps *serverDependencies, mongoClient *mongoinfra.Client) {
	// Model config uses plain MongoDB repos — no Redis cache.
	// System config (sysConfigCacheRepo) is separately cached for admin settings.
	modelRepo := mongoinfra.NewModelConfigRepository(mongoClient.DB())
	defaultRepo := mongoinfra.NewModelDefaultRepository(mongoClient.DB())
	deps.modelCfg = modelcfg.NewProvider(modelRepo, defaultRepo, deps.vaultClient)
	// Wire the invite base URL resolver so admin overrides (INVITE_BASE_URL)
	// in /admin/settings take precedence over the env var / default.
	logic.SetSysConfigRepository(mongoinfra.NewSystemConfigRepository(mongoClient.DB()))
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

// initTaskService creates the task service early (before initAgentEngine)
// so the save_task_result ADK tool has a non-nil TaskRunService dep.
// The queue repo is injected later by initTaskQueue after Redis connects.
func initTaskService(deps *serverDependencies, mongoClient *mongoinfra.Client) {
	deps.taskRepo = mongoinfra.NewTaskDefRepository(mongoClient.DB())
	deps.taskService = task_svc.NewService(
		deps.taskRepo,
		mongoinfra.NewTaskRunRepository(mongoClient.DB()),
		nil, // queue wired later by initTaskQueue
	)
}

// initPII constructs the PII redactor (SPEC-068) before the auditor and KB
// service so both can be injected. enabled reads the `pii_redaction_enabled`
// switch (cache→DB, default true).
func initPII(deps *serverDependencies) {
	enabled := func() bool {
		if deps.sysConfigCacheRepo == nil {
			return true
		}
		cfg, err := deps.sysConfigCacheRepo.Get(context.Background(), "pii_redaction_enabled")
		if err != nil || cfg == nil {
			return true
		}
		return !strings.EqualFold(strings.TrimSpace(cfg.Value), "false")
	}
	deps.piiEnabled = enabled
	deps.piiRedactor = pii.New(pii.Config{
		AnalyzerURL:   os.Getenv("PRESIDIO_ANALYZER_URL"),
		AnonymizerURL: os.Getenv("PRESIDIO_ANONYMIZER_URL"),
		Enabled:       enabled,
	})
}

func initAgentEngine(deps *serverDependencies) {
	deps.secAuditor = security.NewAuditor(nil)
	if deps.piiRedactor != nil {
		deps.secAuditor.SetRedactor(deps.piiRedactor)
	}
	// Wire the text auditor into the model provider so every internal
	// (non-runtime) LLM call — compaction/enhance/intent/relevance/kb — gets
	// input/output text auditing (SPEC-068). Tool-call audit is intentionally
	// not wired here: internal LLM calls expose no tools.
	if deps.modelCfg != nil {
		deps.modelCfg.SetAuditor(deps.secAuditor)
	}
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
		// Priority: admin model list → legacy sysconfig/env fallback.
		cfg := deps.modelCfg.EmbeddingConfig()
		modelURL := cfg.BaseURL
		modelName := cfg.Model

		// Model list takes priority.
		if emb, err := deps.modelCfg.GetDefaultEmbeddingModel(ctx); err == nil && emb != nil && emb.BaseURL != "" {
			modelURL = emb.BaseURL
			modelName = emb.Name
			if cfg.APIKey == "" {
				cfg.APIKey = emb.APIKey
			}
		}
		if modelURL == "" {
			return nil, nil
		}
		cfg.BaseURL = modelURL
		cfg.Model = modelName
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

// initMetrics creates the unified SPEC-072 metrics component (stats_hourly
// hourly counter + reader + llm recorder). Runs BEFORE initServices so the
// artifact storage (built in initArtifacts, also before initServices) can
// attach the counter and the tool deps (built inside initServices) can capture
// a fully-initialized artifact storage — a nil capture here surfaces as a
// save_artifact panic (nil *Storage receiver).
func initMetrics(deps *serverDependencies, mongoClient *mongoinfra.Client) {
	deps.metricsCounter = metrics.NewCounter(mongoClient.DB(), 5*time.Second)
	deps.metricsReader = metrics.NewReader(mongoClient.DB())
	deps.llmRecorder = llmstats.NewRecorder(deps.metricsCounter)
}

// initRedis connects to Redis and builds the LLM cache. Runs BEFORE
// initKnowledgeBase so cachedEmbedFn captures a non-nil cache — previously the
// connection was established inside initTaskQueue (after initKnowledgeBase),
// so the KB embedding cache was silently nil and never hit. Redis failure
// degrades: cache/queue stay disabled, matching the old behavior.
func initRedis(deps *serverDependencies, cfg *config.Config, logger *zap.Logger) {
	redisClient, redisErr := redis.NewClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if redisErr != nil {
		logger.Warn("Failed to connect to Redis — cache/queue disabled", zap.Error(redisErr))
		return
	}
	deps.redisClient = redisClient
	deps.llmCache = llmcache.New(redisClient.Client())
	// Embedding cache TTL reads the live system config on every Set (hot-reload).
	// sysConfigCacheRepo is built early in initServer, before initRedis, and is
	// nil-safe when Redis config backend is unavailable.
	deps.llmCache.SetEmbeddingTTLProvider(func(ctx context.Context) time.Duration {
		if deps.sysConfigCacheRepo == nil {
			return 0 // → default (1h)
		}
		cfgv, err := deps.sysConfigCacheRepo.Get(ctx, "embedding_cache_ttl")
		if err != nil || cfgv == nil {
			return 0
		}
		secs, err := strconv.Atoi(strings.TrimSpace(cfgv.Value))
		if err != nil || secs <= 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	})
}

func initServices(deps *serverDependencies, mongoClient *mongoinfra.Client, logger *zap.Logger) {
	deps.sessionManager = chat.NewManager(mongoinfra.NewSessionRepository(mongoClient.DB()), 24*time.Hour)
	deps.sessionRepo = mongoinfra.NewSessionRepository(mongoClient.DB())
	// SPEC-072: wrap the runtime (chat) LLM with actual-usage recording so the
	// main chat path's token/call counts feed token_tokens + llm_calls (the
	// biggest call point was previously unrecorded).
	if deps.modelCfg != nil {
		deps.modelCfg.SetUsageRecorder(deps.llmRecorder)
	}

	// SPEC-062/066/067: System-level compaction LLM (baked into the shared ADK
	// SessionService). It is a single system LLM built lazily as a singleton:
	// on the very first boot with no model configured it fails silently and
	// retries on the next use — startup never aborts because of a missing model.
	compactionLLM := modelcfg.NewLazyLLM(deps.modelCfg, modelcfg.UseCaseCompaction)

	deps.adkSessions = adksession.NewService(mongoClient.DB()).WithCompaction(
		adksession.CompactionConfig{
			MaxEvents:  100,
			MaxTokens:  4000, // static fallback; MaxTokensFn overrides it dynamically
			KeepRecent: 20,
			// SPEC-067 follow-up: derive the trigger threshold from the
			// compaction model's context length (50%) instead of hardcoding.
			MaxTokensFn: deps.modelCfg.CompactionMaxTokens,
		},
		adksession.NewLLMSummarizer(compactionLLM),
	)

	initMemoryBackend(deps, mongoClient, compactionLLM, logger)

	deps.apiCollectionSvc = apicollectionsvc.NewService(mongo.NewAPICollectionRepo(deps.mongoClient.DB()))

	// SPEC-089: human-in-the-loop channel. The hub is shared by the SSE handler
	// and the blocking gate; the gate is injected into the file_delete /
	// dir_delete / ask_user tools. Timeout defaults to 5 minutes (deny on
	// timeout).
	deps.humanHub = humanchannel.NewHub()
	deps.humanGate = humanchannel.NewGate(deps.humanHub, 0)

	toolDeps := &adktools.Deps{
		KBService:      deps.kbService,
		SkillConfig:    deps.skillConfigSvc,
		Memory:         deps.memoryService,
		MemoryWriter:   deps.memoryKit,
		AppName:        appName,
		Tasks:          deps.taskService,
		SessionSvc:     deps.sessionManager,
		Artifacts:      deps.artifactStorage,
		APICollections: deps.apiCollectionSvc,
		// SPEC-070: graph search skill deps (nil-safe — tool only registers
		// when GraphRepo is non-nil).
		GraphRepo:  deps.graphRepo,
		VectorRepo: deps.vectorStore,
		EmbedFunc:  deps.kbEmbedFn,
		VecCol:     "kb_chunks",
		Counter:    deps.metricsCounter,
		HumanGate:  deps.humanGate,
	}
	tools, err := adktools.All(toolDeps)
	if err != nil {
		logger.Fatal("Failed to build ADK tools", zap.Error(err))
	}

	// SPEC-062: Build the per-model Runtime registry (lazy create + fingerprint
	// hot-reload). Replaces the single shared Runtime; chat.Service resolves a
	// Runtime per session.ModelID at run time.
	//
	// SPEC-071: `tools` above are the base tools (no invoke_subagent). They are
	// shared as SubAgentTools (trimmed — no sub-agent tool, so sub-agents can't
	// delegate recursively). The sub-agent tool is built after the Registry
	// (its Runner needs the Registry) and appended to the parent tool set via
	// SetTools below.
	deps.registry = adkruntime.NewRegistry(adkruntime.RegistryConfig{
		Provider:       deps.modelCfg,
		SessionService: deps.adkSessions,
		MemoryService:  deps.memoryService,
		Tools:          tools,
		SubAgentTools:  tools,
		Auditor:        deps.secAuditor,
		AppName:        appName,
	})

	// SPEC-071: sub-agent tool — delegate a multi-step subtask to an
	// independent sub-agent (same model, trimmed tools). Built after the
	// Registry so the Runner can reuse it; added to the parent tool set after.
	subRunner := subagent.NewRunner(deps.registry, deps.adkSessions, deps.sessionManager)
	if subTool, sErr := subagent.NewTool(subRunner); sErr == nil {
		deps.registry.SetTools(append(tools, subTool))
	} else {
		logger.Warn("Failed to build sub-agent tool", zap.Error(sErr))
	}

	// Evict stale Runtime entries (not accessed in 30 min) to prevent memory leaks.
	go deps.registry.StartCleanup()

	// Guard: intent classification + relevance check + bounded retry. Redis is
	// injected later (initTaskQueue) once it connects. The retry limit is read
	// from system config `guard.max_retries` (cache-first) with a fallback of 2.
	deps.guardSvc = guard.NewService(deps.modelCfg, nil, 2)
	deps.guardSvc.SetMaxRetriesResolver(func(ctx context.Context) int {
		if deps.sysConfigCacheRepo == nil {
			return 0
		}
		cfg, err := deps.sysConfigCacheRepo.Get(ctx, "guard.max_retries")
		if err != nil || cfg == nil {
			return 0
		}
		n, err := strconv.Atoi(strings.TrimSpace(cfg.Value))
		if err != nil || n <= 0 {
			return 0
		}
		return n
	})

	deps.chatService = chat.NewService(deps.registry, deps.modelCfg, deps.adkSessions, deps.sessionManager, deps.cbRegistry, deps.guardSvc).
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
	deps.enhanceService = enhancesvc.NewService(deps.modelCfg, deps.llmCache)
}

func initSkillConfig(deps *serverDependencies, mongoClient *mongoinfra.Client) {
	skillRepo := mongoinfra.NewSkillConfigRepo(mongoClient.DB())
	deps.skillConfigSvc = skillsvc.NewConfigService(skillRepo)
	deps.skillConfigHandler = handler.NewSkillConfigHandler(deps.skillConfigSvc)
}

func initFeishuConfig(deps *serverDependencies, mongoClient *mongoinfra.Client) {
	deps.feishuCfgRepo = mongoinfra.NewFeishuConfigRepository(mongoClient.DB())
	deps.feishuCfgService = feishu_svc.NewConfigService(deps.feishuCfgRepo, deps.sessionManager, deps.vaultClient)
}

// initBuiltins seeds all built-in system configs and skill configs into
// MongoDB at startup. Existing entries (including user-modified values)
// are never overwritten — only new keys/skills are inserted.
func initBuiltins(deps *serverDependencies, logger *zap.Logger) {
	cfgSvc := configsvc.NewService(deps.sysConfigCacheRepo)
	if err := cfgSvc.SeedBuiltins(context.Background()); err != nil {
		logger.Warn("Failed to seed built-in system configs", zap.Error(err))
	} else {
		logger.Info("Built-in system configs initialized")
	}
	if deps.skillConfigSvc != nil {
		if err := deps.skillConfigSvc.SeedSkills(context.Background()); err != nil {
			logger.Warn("Failed to seed built-in skill configs", zap.Error(err))
		} else {
			logger.Info("Built-in skill configs initialized")
		}
	}
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
	deps.artifactStorage = artifact_svc.NewStorage(fileStore, artifactRepo).WithCounter(deps.metricsCounter)
	deps.workspaceMgr = workspace.NewManager(deps.artifactStorage)
	deps.artifactHandler = handler.NewArtifactHandler(deps.artifactStorage, deps.workspaceMgr)
}

func initKnowledgeBase(deps *serverDependencies, mongoClient *mongoinfra.Client) {
	deps.kbService = knowledge.NewService(mongoinfra.NewKBRepository(mongoClient.DB()))
	deps.kbService.WithRedactor(deps.piiRedactor, deps.piiEnabled)
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
		// SPEC-070: expose the vector store + embed fn for the graph search
		// skill (anchor search + content lookup).
		deps.vectorStore = vectorStore
		deps.kbEmbedFn = knowledge.EmbeddingFunc(kEmbedFn)

		// Auto-create Qdrant collection at startup (like a migration).
		// Vectors are associated with KB doc IDs via payload metadata,
		// enabling shared KB support without vector migration.
		// Dimension priority: DB embedding model config → env → default 768.
		vectorDim := 768
		if deps.modelCfg != nil {
			if emb, err := deps.modelCfg.GetDefaultEmbeddingModel(context.Background()); err == nil && emb != nil && emb.EmbeddingDim > 0 {
				vectorDim = emb.EmbeddingDim
			} else if v := getEnvOrDefaultInt("EMBEDDING_VECTOR_DIM", 0); v > 0 {
				vectorDim = v
			}
		}
		if err := vectorStore.EnsureCollection(context.Background(), "kb_chunks", vectorDim); err != nil {
			log.Printf("[kb] WARNING: failed to ensure Qdrant collection kb_chunks: %v", err)
		}
	}
	// SPEC-070: graph index wires into the KB service when available.
	if deps.graphRepo != nil {
		deps.kbService.WithGraphIndex(deps.graphRepo)
	}
	deps.kbHandler = handler.NewKnowledgeHandler(deps.kbService)
}

// initGraphStore creates the ArcadeDB graph store (SPEC-070). Runs BEFORE
// initAgentEngine so the graph search skill's tool deps can see it. Optional:
// disabled when ARCADE_URI is unset.
func initGraphStore(deps *serverDependencies) {
	arcadeURI := getEnvOrDefault("ARCADE_URI", "")
	if arcadeURI == "" {
		return
	}
	driver, err := arcadedbinfra.NewDriver(context.Background(), arcadeURI,
		getEnvOrDefault("ARCADE_USERNAME", "root"),
		getEnvOrDefault("ARCADE_PASSWORD", ""))
	if err != nil {
		log.Printf("[kb] WARNING: failed to create ArcadeDB driver: %v", err)
		return
	}
	graphStore := arcadedbinfra.NewGraphStore(driver, getEnvOrDefault("ARCADE_DATABASE", "kbgraph"), arcadeHTTPURL(arcadeURI))
	if err := graphStore.EnsureSchema(context.Background()); err != nil {
		log.Printf("[kb] WARNING: failed to ensure ArcadeDB schema: %v", err)
	}
	deps.graphRepo = graphStore
	deps.graphStore = graphStore // SPEC-079: concrete type for the health Ping probe
}

// arcadeHTTPURL derives the ArcadeDB HTTP root from the Bolt URI so the health
// probe can align with the compose healthcheck (wget http://<arcade>:2480/)
// without an extra redundant env var. bolt://arcadedb:7687 → http://arcadedb:2480.
func arcadeHTTPURL(boltURI string) string {
	u, err := url.Parse(boltURI)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	return "http://" + u.Hostname() + ":2480"
}

// initMySQLHealthClient builds the health-check-only MySQL singleton (SPEC-079).
// It reads MYSQL_DSN from the environment (injected by docker-compose) and is
// used ONLY by the health probe — business queries still go through the
// sql_executor skill config DSN, so no connection pool or business handle is
// ever built here. Nil when MYSQL_DSN is unset (MySQL probe then skipped).
func initMySQLHealthClient(deps *serverDependencies, logger *zap.Logger) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		logger.Info("MYSQL_DSN unset — MySQL health probe disabled")
		return
	}
	client, err := mysqlinfra.NewClient(dsn)
	if err != nil {
		logger.Warn("Failed to open MySQL health client", zap.Error(err))
		return
	}
	deps.mysqlHealthClient = client
	logger.Info("MySQL health client initialized (health-check only)")
}

// initHealthService assembles the SPEC-079 dependency health probes. Required
// deps (mongodb/redis/qdrant/vault/seaweedfs/mysql) probe whenever their client
// is non-nil; conditional deps (arcadedb/presidio) probe only when enabled and
// are reported "skipped" otherwise. Every probe is a closure so the service
// stays free of infra imports (DDD layering rule).
func initHealthService(deps *serverDependencies, logger *zap.Logger) {
	probes := []monitor.Probe{}

	if deps.mongoClient != nil {
		probes = append(probes, monitor.Probe{Name: "mongodb", Check: deps.mongoClient.Ping})
	}
	if deps.redisClient != nil {
		probes = append(probes, monitor.Probe{Name: "redis", Check: deps.redisClient.Ping})
	}
	if deps.qdrantClient != nil {
		probes = append(probes, monitor.Probe{Name: "qdrant", Check: deps.qdrantClient.Health})
	}
	if deps.vaultClient != nil {
		probes = append(probes, monitor.Probe{Name: "vault", Check: deps.vaultClient.Ping})
	}
	if deps.swClient != nil {
		probes = append(probes, monitor.Probe{Name: "seaweedfs", Check: deps.swClient.Health})
	}
	if deps.mysqlHealthClient != nil {
		probes = append(probes, monitor.Probe{Name: "mysql", Check: deps.mysqlHealthClient.Ping})
	}

	// Conditional deps (SPEC-079 §5.1.1.2): arcadedb only when ARCADE_URI is
	// configured, presidio only when pii_redaction_enabled is on. Configured-out
	// deps are reported "skipped" and never affect the degraded aggregation.
	if deps.graphStore != nil {
		probes = append(probes, monitor.Probe{Name: "arcadedb", Check: deps.graphStore.Ping})
	}
	if deps.piiRedactor != nil && deps.piiEnabled != nil && deps.piiEnabled() {
		probes = append(probes, monitor.Probe{Name: "presidio", Check: deps.piiRedactor.Health})
	}

	deps.healthService = monitor.NewHealthService(getEnvOrDefault("APP_VERSION", ""), probes...)

	skipped := 0
	if deps.graphStore == nil {
		deps.healthService.MarkSkipped("arcadedb")
		skipped++
	}
	if deps.piiRedactor == nil || deps.piiEnabled == nil || !deps.piiEnabled() {
		deps.healthService.MarkSkipped("presidio")
		skipped++
	}

	logger.Info("Health service initialized",
		zap.Int("probes", len(probes)), zap.Int("skipped", skipped))
}

func initAuditAndNotifications(deps *serverDependencies, mongoClient *mongoinfra.Client) {
	deps.auditService = auditsvc.NewService(mongoinfra.NewAuditRepository(mongoClient.DB()), deps.userRepo)
	deps.auditHandler = handler.NewAuditHandler(deps.auditService)
	deps.notifSvc = notifsvc.NewService(mongoinfra.NewNotificationRepository(mongoClient.DB()))
	deps.notifHandler = handler.NewNotificationHandler(deps.notifSvc)
}

func initTaskQueue(deps *serverDependencies, cfg *config.Config, mongoClient *mongoinfra.Client, logger *zap.Logger) {
	// Redis is connected in initRedis (before initKnowledgeBase, for the LLM
	// cache). Task queue wiring happens here, after the task service exists.
	if deps.redisClient == nil {
		logger.Warn("Redis not connected — task queue disabled")
		return
	}
	redisClient := deps.redisClient

	// Inject Redis into the guard (relevance retry counter) once connected.
	if deps.guardSvc != nil {
		deps.guardSvc.SetRedis(redisClient)
	}

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

	// Wire Redis queue into the already-created task service (created in initTaskService).
	if deps.taskService != nil {
		deps.queueRepo = queue.QueueRepository(taskStream)
		deps.taskService.SetQueueRepo(deps.queueRepo)
	}
	deps.taskHandler = handler.NewTaskHandler(deps.taskService, deps.taskService)
	if deps.kbHandler != nil && deps.queueRepo != nil {
		deps.kbHandler.SetQueueRepo(deps.queueRepo)
	} // same Service implements both contracts

	// Wire task service into KB handler for async indexing.

	// Re-wire the orchestrator now that the task service exists.
	if deps.orchestrator != nil {
		deps.orchestrator = agentlogic.NewOrchestrator(deps.sessionManager, deps.taskService, deps.modelCfg)
	}

	sched := scheduler.New(scheduler.NewTaskCreatorFromService(deps.taskService))
	sched.Start(context.Background())
	// Load scheduled agent tasks from DB
	provider := scheduler.NewScheduleProviderFromRepo(deps.taskRepo)
	sched.SetProvider(provider)
	if n, err := sched.LoadFromDB(context.Background(), provider); err != nil {
		logger.Warn("Scheduler failed to load tasks", zap.Error(err))
	} else {
		logger.Info("Scheduler loaded tasks", zap.Int("count", n))
	}

	// SPEC-063: replace the no-op simpleExecutor stub with a real AgentExecutor
	// that reuses the Runtime.RunAndCollect execution path. The executor owns
	// all DB write-back (status/result/error) and user notification; the pool
	// only consumes + loads the task from DB + applies retry/DLQ.
	executor := agentlogic.NewAgentExecutor(
		deps.registry,    // SPEC-062 per-model Runtime registry
		deps.adkSessions, // ADK session service (identity injection)
		deps.taskService, // run status/result/error (TaskRunService impl)
		deps.notifSvc,    // completion/failure notification
		deps.cbRegistry,  // circuit breaker around Runtime.Run
		deps.guardSvc,    // relevance check + bounded retry
	)
	poolSize := resolveWorkerPoolSize(deps.sysConfigCacheRepo)
	workerPool := worker.NewPool(taskStream, redisClient.Client(), poolSize, executor, deps.taskService)

	// Wire KB index executor: handles kb_index tasks as handler-driven pipeline
	// (not agent-controlled). Uses the configured LLM for semantic chunking and
	// embedding for vectorization (kbService already wired with WithVectorIndex).
	if deps.kbService != nil && deps.modelCfg != nil {
		kbExec := agentlogic.NewKBIndexExecutor(deps.kbService, deps.modelCfg, deps.taskService)
		workerPool.SetKBExecutor(kbExec)
		logger.Info("KB index executor wired to worker pool")
	}

	go func() {
		workerPool.Start(context.Background())
	}()
	logger.Info("Task queue and worker pool started", zap.Int("workers", poolSize))
}

// buildRouteDeps constructs the handler wiring for route registration. All
// HTTP handlers are built here; main.go itself defines no handler funcs.
func buildRouteDeps(deps *serverDependencies, cfg *config.Config, logger *zap.Logger) *handler.RouteDeps {
	cfgSvc := configsvc.NewService(deps.sysConfigCacheRepo)
	rbacRepo := mongoinfra.NewRBACRepository(deps.mongoClient.DB())
	rbacSvc := rbacsvc.NewService(rbacRepo, deps.userRepo)

	var imWebhook http.HandlerFunc
	if deps.imService != nil {
		imWebhook = deps.imService.WebhookHandler()
	}

	var imBindHandler *handler.IMBindHandler
	if deps.mongoClient != nil {
		imBindHandler = handler.NewIMBindHandler(im.NewBindService(mongoinfra.NewIMBindRepository(deps.mongoClient.DB(), deps.vaultClient)))
	}

	toolLister := handler.ToolListerFunc(func() []string {
		names, err := adktools.Names(&adktools.Deps{
			KBService:    deps.kbService,
			SkillConfig:  deps.skillConfigSvc,
			Memory:       deps.memoryService,
			MemoryWriter: deps.memoryKit,
			AppName:      appName,
			Tasks:        deps.taskService,
			SessionSvc:   deps.sessionManager,
			Artifacts:    deps.artifactStorage,
			HumanGate:    deps.humanGate,
		})
		if err != nil {
			return []string{}
		}
		return names
	})

	return &handler.RouteDeps{
		JWTManager:     deps.jwtManager,
		AuditLogger:    deps.auditLogger,
		Auth:           deps.authHandler,
		User:           handler.NewUserHandler(user.NewService(deps.userRepo, user.NewBcryptHasher())),
		RBAC:           handler.NewRBACHandler(rbacSvc),
		RBACService:    rbacSvc,
		ModelConfig:    handler.NewModelConfigHandler(cfgSvc, deps.modelCfg),
		SysConfig:      handler.NewConfigHandler(cfgSvc),
		Memory:         handler.NewMemoryHandler(deps.memoryService, deps.memoryKit.Storage(), appName, deps.userRepo, deps.sessionRepo),
		Chat:           handler.NewChatHandler(deps.chatService),
		HumanChannel:   handler.NewHumanChannelHandler(deps.humanHub, deps.sessionManager),
		Enhance:        handler.NewEnhanceHandler(deps.enhanceService),
		Agent:          handler.NewAgentHandler(deps.orchestrator, deps.taskService, toolLister),
		Session:        handler.NewSessionHandler(deps.sessionManager, deps.adkSessions),
		Artifact:       deps.artifactHandler,
		Knowledge:      deps.kbHandler,
		Audit:          deps.auditHandler,
		Notification:   deps.notifHandler,
		Task:           deps.taskHandler,
		Dashboard:      handler.NewDashboardHandler(deps.kbService, deps.metricsReader),
		IMBind:         imBindHandler,
		SkillConfig:    deps.skillConfigHandler,
		FeishuConfig:   handler.NewFeishuConfigHandler(deps.feishuCfgService),
		APICollection:  handler.NewAPICollectionHandler(deps.apiCollectionSvc),
		APITools:       handler.NewAPIToolsHandler(deps.apiCollectionSvc),
		IMWebhook:      imWebhook,
		HermesURL:      os.Getenv("HERMES_URL"),
		AppName:        appName,
		MetricsCounter: deps.metricsCounter,
		MemoryService:  deps.memoryService,
		HealthService:  deps.healthService,
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

// resolveWorkerPoolSize reads WORKER_POOL_SIZE from the system config cache.
// Falls back to 10 on any error (default pool size).
func resolveWorkerPoolSize(cfgRepo *cache.SysConfigCacheRepo) int {
	ctx := context.Background()
	cfg, err := cfgRepo.Get(ctx, "WORKER_POOL_SIZE")
	if err != nil || cfg == nil || cfg.Value == "" {
		log.Printf("[worker] WORKER_POOL_SIZE not found (err=%v, cfg=%v) — using default 10", err, cfg)
		return 10
	}
	n, err := strconv.Atoi(cfg.Value)
	if err != nil || n < 1 {
		log.Printf("[worker] WORKER_POOL_SIZE invalid value %q — using default 10", cfg.Value)
		return 10
	}
	if n > 100 {
		n = 100
	}
	log.Printf("[worker] WORKER_POOL_SIZE resolved to %d", n)
	return n
}
