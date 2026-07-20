package agent_svc

//go:generate mockery --name AgentService --output ./mocks --outpkg mocks

import (
	"github.com/gin-gonic/gin"
)

// AgentService defines the unified agent service contract.
type AgentService interface {
	HandleChat(c *gin.Context)
	CreateAgentTask(c *gin.Context)
	GetAgentTask(c *gin.Context)
	ListSkills(c *gin.Context)
	SearchSkills(c *gin.Context)
}
