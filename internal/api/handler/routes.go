package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"google.golang.org/adk/memory"

	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/domain/consts"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/service/monitor"
)

// RouteDeps bundles every handler and helper needed to register the full
// HTTP route table. main.go constructs this struct and hands it to
// RegisterAllRoutes, keeping the route topology in one place and out of
// the binary entry point.
type RouteDeps struct {
	JWTManager  *middleware.JWTManager
	AuditLogger *middleware.AuditLogger

	Auth         *AuthHandler
	User         *UserHandler
	Role         *RoleHandler
	ModelConfig  *ModelConfigHandler
	SysConfig    *ConfigHandler
	Memory       *MemoryHandler
	Chat         *ChatHandler
	Enhance      *EnhanceHandler
	Agent        *AgentHandler
	Session      *SessionHandler
	Artifact     *ArtifactHandler
	Knowledge    *KnowledgeHandler
	Audit        *AuditHandler
	APIReview    *APIReviewHandler
	Notification *NotificationHandler
	Task         *TaskHandler
	Dashboard    *DashboardHandler
	IMBind       *IMBindHandler

	// IMWebhook is the raw Feishu webhook handler (http.HandlerFunc). May be nil.
	IMWebhook http.HandlerFunc
	// HermesURL enables the Hermes reverse proxy when non-empty.
	HermesURL string
	// AppName namespaces ADK memory searches.
	AppName string
	// MemoryService is the ADK memory service for /memory/search.
	MemoryService memory.Service
}

// RegisterAllRoutes wires the complete HTTP route table onto the router.
// It mirrors the legacy main.registerAllRoutes but contains no inline
// handler functions — every endpoint delegates to a handler method.
func RegisterAllRoutes(router *gin.Engine, deps *RouteDeps) {
	router.Use(middleware.CORSMiddleware())
	router.Use(middleware.RequestIDMiddleware())
	router.Use(gin.Recovery())
	if deps.AuditLogger != nil {
		router.Use(deps.AuditLogger.AuditMiddleware())
	}

	// Public routes (no auth).
	router.GET("/health", HealthCheck)
	if deps.IMWebhook != nil {
		router.POST("/api/v1/im/feishu/webhook", gin.WrapF(deps.IMWebhook))
	}
	if deps.IMBind != nil {
		imBindGroup := router.Group("/api/v1/im/bind")
		imBindGroup.Use(deps.JWTManager.AuthMiddleware())
		RegisterIMBindRoutes(imBindGroup, deps.IMBind)
	}
	RegisterHermesProxy(router, deps.HermesURL)
	router.GET("/api/v1/system/stats", monitor.Handler())

	// Auth routes (no auth).
	registerAuthRoutes(router.Group("/api/v1/auth"), deps.Auth)

	// Protected API routes.
	api := router.Group("/api/v1")
	api.Use(deps.JWTManager.AuthMiddleware())
	registerAuthProtected(api, deps.Auth)
	registerProtectedAPIRoutes(api, deps)

	// Admin routes (auth).
	admin := router.Group("/api/v1/admin")
	admin.Use(deps.JWTManager.AuthMiddleware())
	registerAdminRoutes(admin, deps.Auth)

	// Feature routes (each guarded by auth middleware).
	registerFeatureRoutes(router, deps)
}

// registerProtectedAPIRoutes registers user/role/model/memory/sysconfig routes
// on the authenticated API group. Extracted to reduce cognitive complexity.
func registerProtectedAPIRoutes(api *gin.RouterGroup, deps *RouteDeps) {
	if deps.User != nil {
		RegisterUserRoutes(api, deps.User)
	}
	if deps.Role != nil {
		RegisterRoleRoutes(api, deps.Role)
	}
	if deps.ModelConfig != nil {
		RegisterModelConfigRoutes(api, deps.ModelConfig)
	}
	if deps.Memory != nil {
		RegisterMemoryRoute(api, deps.Memory)
	}
	if deps.SysConfig != nil {
		RegisterSysConfigRoutes(api, deps.SysConfig)
	}
}

// registerFeatureRoutes registers chat/agent/session/artifact/knowledge/audit/
// apireview/notification/task/dashboard routes. Each section is independently
// guarded by auth middleware. Extracted to reduce cognitive complexity.
func registerFeatureRoutes(router *gin.Engine, deps *RouteDeps) {
	if deps.Chat != nil {
		chatRoutes := router.Group("/api/v1/chat")
		chatRoutes.Use(deps.JWTManager.AuthMiddleware())
		RegisterChatRoutes(chatRoutes, deps.Chat)
		if deps.Enhance != nil {
			RegisterEnhanceRoute(chatRoutes, deps.Enhance)
		}
	}
	if deps.Agent != nil {
		agentRoutes := router.Group("/api/v1/agent")
		agentRoutes.Use(deps.JWTManager.AuthMiddleware())
		RegisterAgentRoutes(agentRoutes, deps.Agent)
	}
	if deps.Session != nil {
		sessionRoutes := router.Group("/api/v1/sessions")
		sessionRoutes.Use(deps.JWTManager.AuthMiddleware())
		RegisterSessionRoutes(sessionRoutes, deps.Session)
	}
	if deps.Artifact != nil {
		registerArtifactRoutes(router, deps.JWTManager, deps.Artifact)
		registerWorkspaceRoutes(router, deps.JWTManager, deps.Artifact)
	}
	if deps.Knowledge != nil {
		registerKnowledgeRoutes(router, deps.JWTManager, deps.Knowledge)
		registerAdminKBRoutes(router, deps.JWTManager, deps.Knowledge)
	}
	if deps.Audit != nil {
		registerAuditRoutes(router, deps.JWTManager, deps.Audit)
	}
	if deps.APIReview != nil {
		registerAPIReviewRoutes(router, deps.JWTManager, deps.APIReview)
	}
	if deps.Notification != nil {
		registerNotificationRoutes(router, deps.JWTManager, deps.Notification)
	}
	if deps.Task != nil {
		registerTaskRoutes(router, deps.JWTManager, deps.Task)
	}
	if deps.Dashboard != nil {
		RegisterDashboardRoutes(router, deps.JWTManager.AuthMiddleware(), deps.Dashboard)
	}
}

// registerWorkspaceRoutes registers workspace file routes.
func registerWorkspaceRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *ArtifactHandler) {
	wsRoutes := router.Group("/api/v1/workspace/:session_id")
	wsRoutes.Use(jwt.AuthMiddleware())
	wsRoutes.GET("/files", h.ListWorkspace)
	wsRoutes.GET("/files/:filename", h.ReadWorkspaceFile)
	wsRoutes.PUT("/files/:filename", h.WriteWorkspaceFile)
}

// registerAdminKBRoutes registers admin KB management routes.
func registerAdminKBRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *KnowledgeHandler) {
	adminKB := router.Group("/api/v1/admin/knowledge")
	adminKB.Use(jwt.AuthMiddleware(), middleware.RequirePermission(model.PermUserManage))
	adminKB.GET("/docs", h.ListAllDocs)
}

func registerAuthRoutes(authGroup *gin.RouterGroup, authHandler *AuthHandler) {
	if authHandler != nil {
		authGroup.POST("/login", authHandler.Login)
		authGroup.POST(consts.PathRegister, authHandler.Register)
		authGroup.GET(consts.PathRegister, authHandler.VerifyInvite)
		authGroup.POST("/complete-registration", authHandler.CompleteRegistration)
	} else {
		authGroup.POST("/login", DBUnavailable)
		authGroup.POST(consts.PathRegister, DBUnavailable)
	}
}

func registerAuthProtected(api *gin.RouterGroup, authHandler *AuthHandler) {
	if authHandler != nil {
		api.POST("/auth/refresh", authHandler.RefreshToken)
		api.GET("/auth/profile", authHandler.GetProfile)
	} else {
		api.POST("/auth/refresh", DBUnavailable)
		api.GET("/auth/profile", DBUnavailable)
	}
}

func registerAdminRoutes(admin *gin.RouterGroup, authHandler *AuthHandler) {
	if authHandler == nil {
		return
	}
	admin.POST("/invites", middleware.RequirePermission(model.PermUserManage), authHandler.CreateInvite)
	admin.GET("/invites", middleware.RequirePermission(model.PermUserManage), authHandler.ListInvites)
	admin.DELETE("/invites/:id", middleware.RequirePermission(model.PermUserManage), authHandler.RevokeInvite)
	admin.PUT("/invites/hmac-secret", middleware.RequirePermission(model.PermSystemConfig), authHandler.UpdateHMACSecret)
}

func registerArtifactRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *ArtifactHandler) {
	artifactRoutes := router.Group("/api/v1/artifacts")
	artifactRoutes.Use(jwt.AuthMiddleware())
	artifactRoutes.POST("/upload", h.Upload)
	artifactRoutes.GET("/:id/download", h.Download)
	artifactRoutes.DELETE("/:id", h.Delete)
	artifactRoutes.GET("", h.ListSession)
}

func registerKnowledgeRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *KnowledgeHandler) {
	kbRoutes := router.Group("/api/v1/knowledge")
	kbRoutes.Use(jwt.AuthMiddleware())
	kbRoutes.POST("/docs", h.UploadDoc)
	kbRoutes.GET("/docs", h.ListDocs)
	kbRoutes.GET("/docs/:id", h.GetDoc)
	kbRoutes.DELETE("/docs/:id", h.DeleteDoc)
	kbRoutes.POST("/docs/:id/chunks", h.AddChunks)
	kbRoutes.GET("/search", h.Search)
}

func registerAuditRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *AuditHandler) {
	auditRoutes := router.Group("/api/v1/admin/audit")
	auditRoutes.Use(jwt.AuthMiddleware(), middleware.RequirePermission(model.PermAuditLogView))
	auditRoutes.GET("/logs", h.ListAuditLogs)
	auditRoutes.POST("/export", h.ExportAuditLogs)
}

func registerAPIReviewRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *APIReviewHandler) {
	apiRevRoutes := router.Group("/api/v1/admin/api-reviews")
	apiRevRoutes.Use(jwt.AuthMiddleware(), middleware.RequirePermission(model.PermAPIConvert))
	apiRevRoutes.GET("", h.ListAPIReviews)
	apiRevRoutes.POST("", h.CreateAPIReview)
	apiRevRoutes.PUT("/:id/approve", h.ApproveAPIReview)
	apiRevRoutes.PUT("/:id/reject", h.RejectAPIReview)
}

func registerNotificationRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *NotificationHandler) {
	notifRoutes := router.Group("/api/v1/notifications")
	notifRoutes.Use(jwt.AuthMiddleware())
	notifRoutes.GET("", h.ListNotifications)
	notifRoutes.GET("/unread-count", h.UnreadCount)
	notifRoutes.PUT("/:id/read", h.MarkRead)
	notifRoutes.PUT("/read-all", h.MarkAllRead)
	notifRoutes.POST("", h.SendNotification)
	notifRoutes.POST("/broadcast", h.BroadcastNotification)
}

func registerTaskRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *TaskHandler) {
	taskRoutes := router.Group("/api/v1/tasks")
	taskRoutes.Use(jwt.AuthMiddleware())
	taskRoutes.POST("", h.CreateTask)
	taskRoutes.GET("", h.ListTasks)
	taskRoutes.GET("/:task_id", h.GetTask)
	taskRoutes.PUT("/:task_id/cancel", h.CancelTask)
	taskRoutes.PUT("/:task_id/pause", h.PauseTask)
	taskRoutes.PUT("/:task_id/resume", h.ResumeTask)
	taskRoutes.GET("/:task_id/artifacts/download", h.DownloadArtifacts)

	adminTasks := router.Group("/api/v1/admin/tasks")
	adminTasks.Use(jwt.AuthMiddleware(), middleware.RequirePermission(model.PermUserManage))
	adminTasks.GET("", h.ListAllTasks)
	adminTasks.PUT("/:task_id/retry", h.RetryTask)
	adminTasks.POST("/batch-cancel", h.BatchCancelTasks)
}
