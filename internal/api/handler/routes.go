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
	Notification *NotificationHandler
	Task         *TaskHandler
	Dashboard    *DashboardHandler
	IMBind       *IMBindHandler
	Stats        *StatsHandler
	SkillConfig  *SkillConfigHandler
	FeishuConfig *FeishuConfigHandler
	RBAC         *RBACHandler
	APICollection *APICollectionHandler
	APITools     *APIToolsHandler
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
		RegisterIMBindRoutes(imBindGroup, deps.IMBind, deps.RBACService)
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
		RegisterModelAdminRoutes(admin, deps.ModelConfig, deps.RBACService)
	}
	registerAdminRoutes(admin, deps.Auth, deps.SysConfig, deps.RBACService)

	// API Collection management (admin + system_admin)
	if deps.APICollection != nil {
		registerAPICollectionRoutes(admin, deps.APICollection, deps.RBACService)
	}
	// SkillConfig (system_admin only)
	if deps.SkillConfig != nil {
		admin.GET("/skills", middleware.RequirePermission(deps.RBACService, model.PermModelEdit), deps.SkillConfig.List)
		admin.GET("/skills/:name", middleware.RequirePermission(deps.RBACService, model.PermModelEdit), deps.SkillConfig.Get)
		admin.PUT("/skills/:name", middleware.RequirePermission(deps.RBACService, model.PermModelEdit), deps.SkillConfig.Upsert)
	}

	// Feature routes (each guarded by auth middleware).
	registerFeatureRoutes(router, deps)

	// Skill API tools (for agent tool calling)
	if deps.APITools != nil {
		registerAPIToolsRoutes(router, deps.APITools, deps.JWTManager, deps.RBACService)
	}

	// RBAC routes
	if deps.RBAC != nil {
		registerRBACRoutes(router, deps.JWTManager, deps.RBAC, deps.RBACService)
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

	if deps.Agent != nil {
		agentRoutes := router.Group("/api/v1/agent")
		agentRoutes.Use(deps.JWTManager.AuthMiddleware(), middleware.RequirePermission(deps.RBACService, model.PermAgentView))
		RegisterAgentRoutes(agentRoutes, deps.Agent)
	}
	if deps.Session != nil {
		sessionRoutes := router.Group("/api/v1/sessions")
		sessionRoutes.Use(deps.JWTManager.AuthMiddleware(), middleware.RequirePermission(deps.RBACService, model.PermChatView))
		RegisterSessionRoutes(sessionRoutes, deps.Session, deps.RBACService)
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
	if deps.Notification != nil {
		registerNotificationRoutes(router, deps.JWTManager, deps.Notification)
	}
	if deps.Task != nil {
		registerTaskRoutes(router, deps.JWTManager, deps.Task, deps.RBACService)
	}
	if deps.Dashboard != nil {
		dashRoutes := router.Group("/api/v1/dashboard")
	dashRoutes.Use(deps.JWTManager.AuthMiddleware(), middleware.RequirePermission(deps.RBACService, model.PermDashboardView))
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
	if authHandler != nil {
		admin.POST("/invites", middleware.RequirePermission(rbacSvc, model.PermInviteCreate), authHandler.CreateInvite)
		admin.GET("/invites", middleware.RequirePermission(rbacSvc, model.PermInviteView), authHandler.ListInvites)
		admin.DELETE("/invites/:id", middleware.RequirePermission(rbacSvc, model.PermInviteCreate), authHandler.RevokeInvite)
		admin.PUT("/invites/hmac-secret", middleware.RequirePermission(rbacSvc, model.PermSystemEdit), authHandler.UpdateHMACSecret)
	}

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
	adminTasks.Use(jwt.AuthMiddleware(), middleware.RequirePermission(rbacSvc, model.PermAgentView))
	adminTasks.GET("", h.ListAllTasks)

	adminTasksWrite := router.Group("/api/v1/admin/tasks")
	adminTasksWrite.Use(jwt.AuthMiddleware(), middleware.RequirePermission(rbacSvc, model.PermAgentEdit))
	adminTasksWrite.PUT("/:task_id/retry", h.RetryTask)
	adminTasksWrite.PATCH("/:id/scheduled-enabled", h.ToggleScheduledEnabled)
	adminTasksWrite.POST("/:run_id/cancel", h.CancelRun)

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

func registerRBACRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *RBACHandler, rbacSvc *rbacsvc.Service) {
	// Public: any logged-in user can check their own permissions
	rbacPublic := router.Group("/api/v1/rbac")
	rbacPublic.Use(jwt.AuthMiddleware())
	rbacPublic.GET("/me/permissions", h.MyPermissions)

	// Admin: role/permission management
	rbacAdmin := router.Group("/api/v1/admin")
	rbacAdmin.Use(jwt.AuthMiddleware())

	view := rbacAdmin.Group("/rbac")
	view.Use(middleware.RequirePermission(rbacSvc, model.PermRBACView))
	view.GET("/roles", h.ListRoles)
	view.GET("/roles/:id", h.GetRole)
	view.GET("/roles/:id/available-parents", h.AvailableParents)
	view.GET("/permissions", h.ListPermissions)
	view.GET("/permissions/:id", h.GetPermission)
	view.GET("/roles/:id/permissions", h.ListRolePermissions)
	view.GET("/roles/:id/effective-permissions", h.EffectivePermissions)

	manage := rbacAdmin.Group("/rbac")
	manage.Use(middleware.RequirePermission(rbacSvc, model.PermRBACManage))
	manage.POST("/roles", h.CreateRole)
	manage.PUT("/roles/:id", h.UpdateRole)
	manage.DELETE("/roles/:id", h.DeleteRole)
	manage.DELETE("/permissions/:id", h.DeletePermission)
	manage.POST("/roles/:id/permissions", h.AddRolePermission)
	manage.DELETE("/roles/:id/permissions/:permId", h.RemoveRolePermission)

	// User-role associations (part of user management, not rbac:manage)
	admin := rbacAdmin
	admin.GET("/users/:userId/rbac-roles", middleware.RequirePermission(rbacSvc, model.PermUserView), h.ListUserRoles)
	admin.POST("/users/:userId/rbac-roles", middleware.RequirePermission(rbacSvc, model.PermUserEdit), h.AddUserRole)
	admin.DELETE("/users/:userId/rbac-roles/:id", middleware.RequirePermission(rbacSvc, model.PermUserEdit), h.RemoveUserRole)
}



func registerAPICollectionRoutes(admin *gin.RouterGroup, h *APICollectionHandler, rbacSvc *rbacsvc.Service) {
	apiCol := admin.Group("/api-collections")
	apiCol.GET("", middleware.RequirePermission(rbacSvc, model.PermAPICollectionView), h.List)
	apiCol.POST("", middleware.RequirePermission(rbacSvc, model.PermAPICollectionEdit), h.Create)
	apiCol.GET("/:id", middleware.RequirePermission(rbacSvc, model.PermAPICollectionView), h.Get)
	apiCol.PUT("/:id", middleware.RequirePermission(rbacSvc, model.PermAPICollectionEdit), h.Update)
	apiCol.DELETE("/:id", middleware.RequirePermission(rbacSvc, model.PermAPICollectionDelete), h.Delete)
	apiCol.POST("/:id/approve", middleware.RequirePermission(rbacSvc, model.PermAPICollectionApprove), h.Approve)
}


func registerAPIToolsRoutes(router *gin.Engine, h *APIToolsHandler, jwt *middleware.JWTManager, rbacSvc *rbacsvc.Service) {
	tools := router.Group("/api/v1/tools/api")
	tools.Use(jwt.AuthMiddleware(), middleware.RequirePermission(rbacSvc, model.PermChatView))
	tools.GET("/search", h.Search)
	tools.GET("/summary", h.Summary)
	tools.GET("/method", h.Method)
	tools.POST("/call", h.Call)
}
