package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/service/chat"
	"github.com/luoxiaojun1992/data-agent/internal/service/knowledge"
	"github.com/luoxiaojun1992/data-agent/internal/service/task"
)

type DashboardHandler struct {
	taskService    *task.Service
	taskHandler    *TaskHandler
	sessionManager *chat.Manager
	kbService      *knowledge.Service
}

func NewDashboardHandler(taskSvc *task.Service, taskHdl *TaskHandler, sessMgr *chat.Manager, kbSvc *knowledge.Service) *DashboardHandler {
	return &DashboardHandler{
		taskService: taskSvc, taskHandler: taskHdl,
		sessionManager: sessMgr, kbService: kbSvc,
	}
}

func RegisterDashboardRoutes(router *gin.Engine, midd gin.HandlerFunc, h *DashboardHandler) {
	router.GET("/api/v1/admin/dashboard", midd, h.Get)
}

func (h *DashboardHandler) Get(c *gin.Context) {
	userID := c.GetString("user_id")

	tasks, _ := h.taskService.ListAllTasks(userID)
	sessions, _ := h.sessionManager.ListByUser(userID)
	docs, _ := h.kbService.ListAllDocs()

	c.JSON(http.StatusOK, gin.H{
		"tasks":    tasks,
		"sessions": sessions,
		"docs":     docs,
	})
}
