package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"google.golang.org/adk/memory"

	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/domain/consts"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/infra/metrics"
	"github.com/luoxiaojun1992/data-agent/internal/service/monitor"
	rbacsvc "github.com/luoxiaojun1992/data-agent/internal/service/rbac"
)

// RouteDeps bundles every handler and helper needed to register the full
// HTTP route table. main.go constructs this struct and hands it to
// RegisterAllRoutes, keeping the route topology in one place and out of
// the binary entry point.
type RouteDeps struct {
	JWTManager  *middleware.JWTManager
	AuditLogger *middleware.AuditLogger

	Auth          *AuthHandler
	User          *UserHandler
	ModelConfig   *ModelConfigHandler
	SysConfig     *ConfigHandler
	Memory        *MemoryHandler
	Chat          *ChatHandler
	HumanChannel  *HumanChannelHandler
	Enhance       *EnhanceHandler
	Session       *SessionHandler
	Artifact      *ArtifactHandler
	Knowledge     *KnowledgeHandler
	Audit         *AuditHandler
	Notification  *NotificationHandler
	Task          *TaskHandler
	Dashboard     *DashboardHandler
	IMBind        *IMBindHandler
	SkillConfig   *SkillConfigHandler
	FeishuConfig  *FeishuConfigHandler
	RBAC          *RBACHandler
	APICollection *APICollectionHandler
	RBACService   *rbacsvc.Service // for RequirePermission middleware

	// IMWebhook is the raw Feishu webhook handler (http.HandlerFunc). May be nil.
	IMWebhook http.HandlerFunc
	// HermesURL enables the Hermes reverse proxy when non-empty.
	HermesURL string
	// AppName namespaces ADK memory searches.
	AppName string
	// MemoryService is the ADK memory service for /memory/search.
	MemoryService memory.Service
	// MetricsCounter increments api_calls for /api/v1/* requests (SPEC-072).
	MetricsCounter metrics.Counter
	// HealthService powers the enhanced /health and /api/v1/health endpoints
	// (SPEC-079). May be nil — both routes fall back to the legacy HealthCheck.
	HealthService *monitor.HealthService
}

// RegisterAllRoutes wires the complete HTTP route table onto the router.
// It mirrors the legacy main.registerAllRoutes but contains no inline
// handler functions — every endpoint delegates to a handler method.
func RegisterAllRoutes(router *gin.Engine, deps *RouteDeps) {
	router.Use(middleware.CORSMiddleware())
	router.Use(middleware.RequestIDMiddleware())
	router.Use(gin.Recovery())
	// SPEC-072: count every /api/v1/* request (api_calls).
	router.Use(middleware.MetricsMiddleware(deps.MetricsCounter))
	if deps.AuditLogger != nil {
		router.Use(deps.AuditLogger.AuditMiddleware())
	}

	// Public routes (no auth).
	// SPEC-079: enhanced dependency health check (no auth). Both /health and
	// /api/v1/health serve the same payload; "degraded" is carried in the
	// status field (not the HTTP code) so the frontend always gets the
	// per-dependency detail. Falls back to the legacy HealthCheck when no
	// HealthService is wired.
	healthHandler := NewHealthHandler(deps.HealthService)
	router.GET("/health", healthHandler.Check)
	router.GET("/api/v1/health", healthHandler.Check)
	if deps.IMWebhook != nil {
		router.POST("/api/v1/im/feishu/webhook", gin.WrapF(deps.IMWebhook))
	}
	if deps.IMBind != nil {
		imBindGroup := router.Group("/api/v1/im/bind")
		imBindGroup.Use(deps.JWTManager.AuthMiddleware())
		RegisterIMBindRoutes(imBindGroup, deps.IMBind, deps.RBACService)
	}
	RegisterHermesProxy(router, deps.HermesURL)

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

	// RBAC routes
	if deps.RBAC != nil {
		registerRBACRoutes(router, deps.JWTManager, deps.RBAC, deps.RBACService)
	}
}

// registerProtectedAPIRoutes registers user/role/model/memory/sysconfig routes
// on the authenticated API group. Extracted to reduce cognitive complexity.
func registerProtectedAPIRoutes(api *gin.RouterGroup, deps *RouteDeps) {
	if deps.User != nil {
		RegisterUserRoutes(api, deps.User, deps.RBACService)
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
		if deps.HumanChannel != nil {
			// SPEC-089: human-in-the-loop channel shares the chat permission
			// (ordinary users) — no extra RBAC beyond the group middleware.
			RegisterHumanChannelRoutes(chatRoutes, deps.HumanChannel)
		}
	}

	if deps.Session != nil {
		sessionRoutes := router.Group("/api/v1/sessions")
		sessionRoutes.Use(deps.JWTManager.AuthMiddleware(), middleware.RequirePermission(deps.RBACService, model.PermChatView))
		RegisterSessionRoutes(sessionRoutes, deps.Session, deps.RBACService)
	}
	if deps.Artifact != nil {
		registerArtifactRoutes(router, deps.JWTManager, deps.Artifact, deps.RBACService)
	}
	if deps.Knowledge != nil {
		registerKnowledgeRoutes(router, deps.JWTManager, deps.Knowledge, deps.RBACService)
	}
	if deps.Audit != nil {
		registerAuditRoutes(router, deps.JWTManager, deps.Audit, deps.RBACService)
	}
	if deps.Notification != nil {
		registerNotificationRoutes(router, deps.JWTManager, deps.Notification, deps.RBACService)
	}
	if deps.Task != nil {
		registerTaskRoutes(router, deps.JWTManager, deps.Task, deps.RBACService)
	}
	if deps.Dashboard != nil {
		RegisterDashboardRoutes(router, deps.JWTManager.AuthMiddleware(), deps.Dashboard, deps.RBACService)
	}
	if deps.FeishuConfig != nil {
		registerFeishuRoutes(router, deps.JWTManager, deps.FeishuConfig, deps.RBACService)
	}
}

// registerWorkspaceRoutes was removed (SPEC-084): workspace files are accessed
// by the agent via the fsops local directory, not over HTTP.

func registerAuthRoutes(authGroup *gin.RouterGroup, authHandler *AuthHandler) {
	if authHandler != nil {
		authGroup.POST("/login", authHandler.Login)
		authGroup.GET(consts.PathRegister, authHandler.VerifyInvite)
		authGroup.POST("/complete-registration", authHandler.CompleteRegistration)
	} else {
		authGroup.POST("/login", DBUnavailable)
	}
}

func registerAuthProtected(api *gin.RouterGroup, authHandler *AuthHandler) {
	if authHandler != nil {
		api.POST("/auth/refresh", authHandler.RefreshToken)
		api.GET("/auth/profile", authHandler.GetProfile)
		api.POST("/auth/change-password", authHandler.ChangePassword)
	} else {
		api.POST("/auth/refresh", DBUnavailable)
		api.GET("/auth/profile", DBUnavailable)
		api.POST("/auth/change-password", DBUnavailable)
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

func registerArtifactRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *ArtifactHandler, rbacSvc *rbacsvc.Service) {
	artifactRoutes := router.Group("/api/v1/artifacts")
	artifactRoutes.Use(jwt.AuthMiddleware())
	artifactRoutes.POST("/upload", middleware.RequirePermission(rbacSvc, model.PermArtifactView), h.Upload)
	artifactRoutes.GET("/:id/download", middleware.RequirePermission(rbacSvc, model.PermArtifactView), h.Download)
	artifactRoutes.GET("/:id/download-url", middleware.RequirePermission(rbacSvc, model.PermArtifactView), h.DownloadURL)
	artifactRoutes.DELETE("/:id", middleware.RequirePermission(rbacSvc, model.PermArtifactDelete), h.Delete)
	artifactRoutes.GET("", middleware.RequirePermission(rbacSvc, model.PermArtifactView), h.ListSession)
	artifactRoutes.GET("/user", middleware.RequirePermission(rbacSvc, model.PermArtifactView), h.ListUser)
}

func registerKnowledgeRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *KnowledgeHandler, rbacSvc *rbacsvc.Service) {
	kbRoutes := router.Group("/api/v1/knowledge")
	kbRoutes.Use(jwt.AuthMiddleware())
	kbRoutes.POST("/docs", middleware.RequirePermission(rbacSvc, model.PermKBUpload), h.UploadDoc)
	kbRoutes.GET("/docs", middleware.RequirePermission(rbacSvc, model.PermKBView), h.ListDocs)
	kbRoutes.GET("/docs/:id", middleware.RequirePermission(rbacSvc, model.PermKBView), h.GetDoc)
	kbRoutes.PUT("/docs/:id/public", middleware.RequirePermission(rbacSvc, model.PermKBUpload), h.SetPublicFlag)
	kbRoutes.DELETE("/docs/:id", middleware.RequirePermission(rbacSvc, model.PermKBDelete), h.DeleteDoc)
	kbRoutes.GET("/search", middleware.RequirePermission(rbacSvc, model.PermKBView), h.Search)
}

func registerAuditRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *AuditHandler, rbacSvc *rbacsvc.Service) {
	auditRoutes := router.Group("/api/v1/admin/audit")
	auditRoutes.Use(jwt.AuthMiddleware(), middleware.RequirePermission(rbacSvc, model.PermAuditView))
	auditRoutes.GET("/logs", h.ListAuditLogs)
	auditRoutes.POST("/export", h.ExportAuditLogs)
}

func registerNotificationRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *NotificationHandler, rbacSvc *rbacsvc.Service) {
	notifRoutes := router.Group("/api/v1/notifications")
	notifRoutes.Use(jwt.AuthMiddleware())
	// 读取侧：查自己的通知，JWT-only（白名单，不加 RBAC）。
	notifRoutes.GET("", h.ListNotifications)
	notifRoutes.GET("/unread-count", h.UnreadCount)
	notifRoutes.PUT("/:id/read", h.MarkRead)
	notifRoutes.PUT("/read-all", h.MarkAllRead)
	// 写侧：定向发送（user 级）/ 广播（admin 级）需 RBAC。
	notifRoutes.POST("", middleware.RequirePermission(rbacSvc, model.PermNotificationSend), h.SendNotification)
	notifRoutes.POST("/broadcast", middleware.RequirePermission(rbacSvc, model.PermNotificationBroadcast), h.BroadcastNotification)
}

func registerTaskRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *TaskHandler, rbacSvc *rbacsvc.Service) {
	taskRoutes := router.Group("/api/v1/tasks")
	taskRoutes.Use(jwt.AuthMiddleware())
	taskRoutes.POST("", middleware.RequirePermission(rbacSvc, model.PermAgentCreate), h.CreateTask)
	taskRoutes.GET("", middleware.RequirePermission(rbacSvc, model.PermAgentView), h.ListTasks)
	taskRoutes.GET("/:task_id", middleware.RequirePermission(rbacSvc, model.PermAgentView), h.GetTask)
	taskRoutes.POST("/:task_id/run", middleware.RequirePermission(rbacSvc, model.PermAgentEdit), h.CreateRun)
	taskRoutes.GET("/:task_id/runs", middleware.RequirePermission(rbacSvc, model.PermAgentView), h.ListRuns)
	taskRoutes.GET("/:task_id/runs/:run_id", middleware.RequirePermission(rbacSvc, model.PermAgentView), h.GetRun)
	taskRoutes.PUT("/:task_id/cancel", middleware.RequirePermission(rbacSvc, model.PermAgentEdit), h.CancelTask)
	taskRoutes.GET("/:task_id/artifacts/download", middleware.RequirePermission(rbacSvc, model.PermAgentView), h.DownloadArtifacts)

	// Scheduled-enabled toggle — also used by the /agent page.
	adminTasksWrite := router.Group("/api/v1/admin/tasks")
	adminTasksWrite.Use(jwt.AuthMiddleware(), middleware.RequirePermission(rbacSvc, model.PermAgentEdit))
	adminTasksWrite.PATCH("/:id/scheduled-enabled", h.ToggleScheduledEnabled)

	// Standalone run endpoint — useful for the run-detail page where the
	// client only has run_id (task_id is in URL state).
	runRoutes := router.Group("/api/v1/runs")
	runRoutes.Use(jwt.AuthMiddleware(), middleware.RequirePermission(rbacSvc, model.PermAgentView))
	runRoutes.GET("/:run_id", h.GetRun)
}

func registerFeishuRoutes(router *gin.Engine, jwt *middleware.JWTManager, h *FeishuConfigHandler, rbacSvc *rbacsvc.Service) {
	feishuRoutes := router.Group("/api/v1/im/feishu/configs")
	feishuRoutes.Use(jwt.AuthMiddleware())
	feishuRoutes.POST("", middleware.RequirePermission(rbacSvc, model.PermIMEdit), h.Create)
	feishuRoutes.GET("", middleware.RequirePermission(rbacSvc, model.PermIMView), h.List)
	feishuRoutes.GET("/:id", middleware.RequirePermission(rbacSvc, model.PermIMView), h.Get)
	feishuRoutes.PUT("/:id", middleware.RequirePermission(rbacSvc, model.PermIMEdit), h.Update)
	feishuRoutes.DELETE("/:id", middleware.RequirePermission(rbacSvc, model.PermIMDelete), h.Delete)
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
	view.GET("/parent-candidates", h.ListParentCandidates)
	view.GET("/permissions", h.ListPermissions)
	view.GET("/permissions/:id", h.GetPermission)
	view.GET("/roles/:id/permissions", h.ListRolePermissions)
	view.GET("/roles/:id/effective-permissions", h.EffectivePermissions)

	manage := rbacAdmin.Group("/rbac")
	manage.Use(middleware.RequirePermission(rbacSvc, model.PermRBACManage))
	manage.POST("/roles", h.CreateRole)
	manage.PUT("/roles/:id", h.UpdateRole)
	manage.DELETE("/roles/:id", h.DeleteRole)
	manage.POST("/permissions", h.CreatePermission)
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
