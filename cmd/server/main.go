// Package main is the entry point for the DataAgent server.
// SPEC-058: main.go is a thin init+lifecycle shell. All HTTP handlers live
// in internal/api/handler, cross-service orchestration in internal/logic/agent,
// service wiring in wire.go, and route topology in handler.RegisterAllRoutes.
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
	"github.com/luoxiaojun1992/data-agent/internal/adk/memoryx"
	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
	adkruntime "github.com/luoxiaojun1992/data-agent/internal/adk/runtime"
	adksession "github.com/luoxiaojun1992/data-agent/internal/adk/session"
	"github.com/luoxiaojun1992/data-agent/internal/api/handler"
	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/config"
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
	agentlogic "github.com/luoxiaojun1992/data-agent/internal/logic/agent"
	"github.com/luoxiaojun1992/data-agent/internal/logic/workspace"
	"github.com/luoxiaojun1992/data-agent/internal/queue"
	apireview "github.com/luoxiaojun1992/data-agent/internal/service/apireview"
	artifact_svc "github.com/luoxiaojun1992/data-agent/internal/service/artifact"
	auditsvc "github.com/luoxiaojun1992/data-agent/internal/service/audit"
	"github.com/luoxiaojun1992/data-agent/internal/service/chat"
	enhancesvc "github.com/luoxiaojun1992/data-agent/internal/service/enhance"
	"github.com/luoxiaojun1992/data-agent/internal/service/im"
	"github.com/luoxiaojun1992/data-agent/internal/service/knowledge"
	notifsvc "github.com/luoxiaojun1992/data-agent/internal/service/notification"
	task_svc "github.com/luoxiaojun1992/data-agent/internal/service/task"
	"go.uber.org/zap"

	"google.golang.org/adk/memory"
)

// appName namespaces ADK sessions, memory entries, and tool registration.
const appName = "data-agent"

func main() {
	cfg, logger, mongoClient, deps := initServer()
	defer cleanup(logger, mongoClient, &deps)

	router := buildRouter(cfg)
	routeDeps := buildRouteDeps(&deps, cfg, logger)
	handler.RegisterAllRoutes(router, routeDeps)
	startServer(router, cfg, logger)
}

// serverDependencies holds every constructed service, handler, and infra client.
type serverDependencies struct {
	mongoClient      *mongoinfra.Client
	userRepo         *mongoinfra.UserRepository
	roleRepo         *mongoinfra.RoleRepository
	systemConfigRepo *mongoinfra.SystemConfigRepository
	vaultClient      *vaultinfra.Client
	authHandler      *handler.AuthHandler
	qdrantClient     *qdrantinfra.Client
	swClient         *seaweedfs.Client
	jwtManager       *middleware.JWTManager
	auditLogger      *middleware.AuditLogger
	redisClient      *redis.Client
	// ADK + chat wiring (populated by wire.go init functions).
	modelCfg       *modelcfg.Provider
	adkRuntime     *adkruntime.Runtime
	adkSessions    *adksession.Service
	memoryService  memory.Service
	memoryKit      *memoryx.Kit
	sessionManager *chat.Manager
	chatService    *chat.Service
	secAuditor     *security.Auditor
	cbRegistry     *security.CircuitBreakerRegistry
	llmRecorder    *llmstats.Recorder
	llmCache       *llmcache.Cache
	taskStream     *queue.Stream
	orchestrator   *agentlogic.Orchestrator
	enhanceService *enhancesvc.Service
	imService      *im.Service
	// Handlers + services (populated by wire.go init functions).
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

	mongoClient, err := mongoinfra.NewClient(ctx, cfg.Mongo.URI, cfg.Mongo.Database)
	deps.mongoClient = mongoClient
	if err != nil {
		logger.Warn("Failed to connect to MongoDB — server will start without database",
			zap.Error(err), zap.String("uri", cfg.Mongo.URI))
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
	deps.auditLogger = middleware.NewAuditLogger(mongoinfra.NewAuditRepository(mongoClient.DB()))

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
	initEnhance(&deps)
	initIM(&deps)

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

func buildRouter(cfg *config.Config) *gin.Engine {
	if cfg.Log.Level != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	return gin.New()
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
		zap.Int("port", cfg.Server.Port), zap.String("log_level", cfg.Log.Level))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal("Server error", zap.Error(err))
	}
	logger.Info("Server exited gracefully")
}

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
		zap.String("note", "请尽快修改密码！登录后横向提示修改"))
	return nil
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

type simpleExecutor struct {
	taskSvc interface{}
}

func (e *simpleExecutor) Execute(ctx context.Context, t *task.Task) error {
	_ = ctx
	_ = t
	return nil
}
