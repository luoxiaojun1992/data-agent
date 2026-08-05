package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"google.golang.org/adk/memory"

	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/domain/consts"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	rbacsvc "github.com/luoxiaojun1992/data-agent/internal/service/rbac"
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
	Stats        *StatsHandler
	SkillConfig  *SkillConfigHandler
	FeishuConfig *FeishuConfigHandler
	RBAC         *RBACHandler
	RBACService  *rbacsvc.Service // for RequirePermission middleware

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

	// Model public selectors (used by chat for any logged-in user)
	if deps.ModelConfig != nil {
		RegisterModelPublicRoutes(api, deps.ModelConfig, deps.RBACService)
	}

	// Admin routes (auth).
	admin := router.Group("/api/v1/admin")
	admin.Use(deps.JWTManager.AuthMiddleware())
	if deps.ModelConfig != nil {
		RegisterModelAdminRoutes(admin, deps.ModelConfig)
	}
	registerAdminRoutes(admin, deps.Auth, deps.SysConfig, deps.RBACService)

	// Feature routes (each guarded by auth middleware).
	registerFeatureRoutes(router, deps)

	// RBAC routes
	if deps.RBAC != nil {
		registerRBACRoutes(router, deps.JWTManager, deps.RBAC)
	}
}

// registerProtectedAPIRoutes registers user/role/model/memory/sysconfig routes
// on the authenticated API group. Extracted to reduce cognitive complexity.
func registerProtectedAPIRoutes(api *gin.RouterGroup, deps *RouteDeps) {
	if deps.User != nil {
		RegisterUserRoutes(api, deps.User)
	}
	if deps.Memory != nil {
		RegisterMemoryRoute(api, deps.Memory, deps.RBACService)
	}
}

// registerFeatureRoutes registers chat/agent/session/artifact/knowledge/audit/
// apireview/notification/task/dashboard routes. Each section is independently
// guarded by auth middleware. Extracted to reduce cognitive complexity.
func registerFeatureRoutes(router *gin.Engine, deps *RouteDeps) {
	if deps.Chat != nil {
		chatRoutes := router.Group("/api/v1/chat")
		chatRoutes.Use(deps.JWTManager.AuthMiddleware(), middleware.RequirePermission(deps.RBACService, model.PermChatView))
		RegisterChatRoutes(chatRoutes, deps.Chat)
		if deps.Enhance != nil {
			RegisterEnhanceRoute(chatRoutes, deps.Enhance)
		}
	}
	// SkillConfig moved to admin group above

	if deps.Agent != nil {
		agentRoutes := router.Group("/api/v1/agent")
		agentRoutes.Use(deps.JWTManager.AuthMiddleware(), middleware.RequirePermission(deps.RBACService, model.PermAgentView))
		RegisterAgentRoutes(agentRoutes, deps.Agent)
	}
	if deps.Session != nil {
		sessionRoutes := router.Group("/api/v1/sessions")
		sessionRoutes.Use(deps.JWTManager.AuthMiddleware(), middleware.RequirePermission(deps.RBACService, model.PermChatView))
		RegisterSessionRoutes(sessionRoutes, deps.Session)
	}
	if deps.Artifact != nil {
		registerArtifactRoutes(router, deps.JWTManager, deps.Artifact)
		registerWorkspaceRoutes(router, deps.JWTManager, deps.Artifact)
	}
	if deps.Knowledge != nil {
		registerKnowledgeRoutes(router, deps.JWTManager, deps.Knowledge)
		registerAdminKBRoutes(router, deps.JWTManager, deps.Knowledge, deps.RBACService)
	}
	if deps.Audit != nil {
		registerAuditRoutes(router, deps.JWTManager, deps.Audit, deps.RBACService)
	}
	if deps.APIReview != nil {
		registerAPIReviewRoutes(router, deps.JWTManager, deps.APIReview, deps.RBACService)
	}
	if deps.Notification != nil {
		registerNotificationRoutes(router, deps.JWTManager, deps.Notification)
	}
	if deps.Task != nil {
		registerTaskRoutes(router, deps.JWTManager, deps.Task, deps.RBACService)
	}
	if deps.Dashboard != nil {
		RegisterDashboardRoutes(router, deps.JWTManager.AuthMiddleware(), deps.Dashboard)
	}
	if deps.Stats != nil {
		RegisterStatsRoutes(router, deps.JWTManager, deps.Stats, deps.RBACService)
	}
	if deps.FeishuConfig != nil {
		registerFeishuRoutes(router, deps.JWTManager, deps.FeishuConfig)
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
func registerAdminKBRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *KnowledgeHandler, rbacSvc *rbacsvc.Service) {
	adminKB := router.Group("/api/v1/admin/knowledge")
	adminKB.Use(jwt.AuthMiddleware(), middleware.RequirePermission(rbacSvc, model.PermKBDelete))
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

func registerAdminRoutes(admin *gin.RouterGroup, authHandler *AuthHandler, sysConfig *ConfigHandler, rbacSvc *rbacsvc.Service) {
	if authHandler == nil && sysConfig == nil {
		return
	}
	admin.POST("/invites", middleware.RequirePermission(rbacSvc, model.PermInviteCreate), authHandler.CreateInvite)
	admin.GET("/invites", middleware.RequirePermission(rbacSvc, model.PermInviteView), authHandler.ListInvites)
	admin.DELETE("/invites/:id", middleware.RequirePermission(rbacSvc, model.PermInviteCreate), authHandler.RevokeInvite)
	admin.PUT("/invites/hmac-secret", middleware.RequirePermission(rbacSvc, model.PermSystemEdit), authHandler.UpdateHMACSecret)

	// System configuration (system_admin only)
	if sysConfig != nil {
		RegisterSysConfigRoutes(admin, sysConfig, rbacSvc)
	}
}

func registerArtifactRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *ArtifactHandler) {
	artifactRoutes := router.Group("/api/v1/artifacts")
	artifactRoutes.Use(jwt.AuthMiddleware())
	artifactRoutes.POST("/upload", h.Upload)
	artifactRoutes.GET("/:id/download", h.Download)
	artifactRoutes.GET("/:id/download-url", h.DownloadURL)
	artifactRoutes.DELETE("/:id", h.Delete)
	artifactRoutes.GET("", h.ListSession)
	artifactRoutes.GET("/user", h.ListUser)
}

func registerKnowledgeRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *KnowledgeHandler) {
	kbRoutes := router.Group("/api/v1/knowledge")
	kbRoutes.Use(jwt.AuthMiddleware())
	kbRoutes.POST("/docs", h.UploadDoc)
	kbRoutes.GET("/docs", h.ListDocs)
	kbRoutes.GET("/docs/:id", h.GetDoc)
	kbRoutes.PUT("/docs/:id/public", h.SetPublicFlag)
	kbRoutes.DELETE("/docs/:id", h.DeleteDoc)
	kbRoutes.POST("/docs/:id/chunks", h.AddChunks)
	kbRoutes.GET("/search", h.Search)
}

func registerAuditRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *AuditHandler, rbacSvc *rbacsvc.Service) {
	auditRoutes := router.Group("/api/v1/admin/audit")
	auditRoutes.Use(jwt.AuthMiddleware(), middleware.RequirePermission(rbacSvc, model.PermAuditView))
	auditRoutes.GET("/logs", h.ListAuditLogs)
	auditRoutes.POST("/export", h.ExportAuditLogs)
}

func registerAPIReviewRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *APIReviewHandler, rbacSvc *rbacsvc.Service) {
	apiRevRoutes := router.Group("/api/v1/admin/api-reviews")
	apiRevRoutes.Use(jwt.AuthMiddleware(), middleware.RequirePermission(rbacSvc, model.PermAPIReviewView))
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

func registerTaskRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *TaskHandler, rbacSvc *rbacsvc.Service) {
	taskRoutes := router.Group("/api/v1/tasks")
	taskRoutes.Use(jwt.AuthMiddleware())
	taskRoutes.POST("", h.CreateTask)
	taskRoutes.GET("", h.ListTasks)
	taskRoutes.GET("/:task_id", h.GetTask)
	taskRoutes.POST("/:task_id/run", h.CreateRun)
	taskRoutes.GET("/:task_id/runs", h.ListRuns)
	taskRoutes.GET("/:task_id/runs/:run_id", h.GetRun)
	taskRoutes.PUT("/:task_id/cancel", h.CancelTask)
	taskRoutes.PUT("/:task_id/pause", h.PauseTask)
	taskRoutes.PUT("/:task_id/resume", h.ResumeTask)
	taskRoutes.GET("/:task_id/artifacts/download", h.DownloadArtifacts)

	adminTasks := router.Group("/api/v1/admin/tasks")
	adminTasks.Use(jwt.AuthMiddleware(), middleware.RequirePermission(rbacSvc, model.PermTaskView))
	adminTasks.GET("", h.ListAllTasks)
	adminTasks.PUT("/:task_id/retry", h.RetryTask)
	adminTasks.POST("/batch-cancel", h.BatchCancelTasks)

	// Standalone run endpoint — useful for the run-detail page where the
	// client only has run_id (task_id is in URL state).
	runRoutes := router.Group("/api/v1/runs")
	runRoutes.Use(jwt.AuthMiddleware())
	runRoutes.GET("/:run_id", h.GetRun)
}

func registerFeishuRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *FeishuConfigHandler) {
	feishuRoutes := router.Group("/api/v1/im/feishu/configs")
	feishuRoutes.Use(jwt.AuthMiddleware())
	feishuRoutes.POST("", h.Create)
	feishuRoutes.GET("", h.List)
	feishuRoutes.GET("/:id", h.Get)
	feishuRoutes.PUT("/:id", h.Update)
	feishuRoutes.DELETE("/:id", h.Delete)
}

func registerRBACRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *RBACHandler) {
	rbac := router.Group("/api/v1/rbac")
	rbac.Use(jwt.AuthMiddleware())

	// Roles
	rbac.GET("/roles", h.ListRoles)
	rbac.GET("/roles/:id", h.GetRole)
	rbac.POST("/roles", h.CreateRole)
	rbac.PUT("/roles/:id", h.UpdateRole)
	rbac.DELETE("/roles/:id", h.DeleteRole)
	rbac.GET("/roles/:id/available-parents", h.AvailableParents)

	// Permissions
	rbac.GET("/permissions", h.ListPermissions)
	rbac.GET("/permissions/:id", h.GetPermission)
	rbac.DELETE("/permissions/:id", h.DeletePermission)

	// Role-permission associations
	rbac.GET("/roles/:id/permissions", h.ListRolePermissions)
	rbac.POST("/roles/:id/permissions", h.AddRolePermission)
	rbac.DELETE("/roles/:id/permissions/:permId", h.RemoveRolePermission)

	// Effective permissions
	rbac.GET("/roles/:id/effective-permissions", h.EffectivePermissions)
	rbac.GET("/me/permissions", h.MyPermissions)

	// User-role associations (admin)
	admin := router.Group("/api/v1/admin")
	admin.Use(jwt.AuthMiddleware())
	admin.GET("/users/:userId/rbac-roles", h.ListUserRoles)
	admin.POST("/users/:userId/rbac-roles", h.AddUserRole)
	admin.DELETE("/users/:userId/rbac-roles/:id", h.RemoveUserRole)
}
