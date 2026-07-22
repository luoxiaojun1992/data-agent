// Package agent contains the orchestration layer for agent use cases.
// The orchestrator combines multiple services (chat session, task) to
// fulfill use cases that span service boundaries, eliminating same-layer
// service dependencies. It depends on domain contracts, not on concrete
// service implementations.
package agent

import (
	"context"
	"fmt"

	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
)

// Orchestrator coordinates cross-service agent use cases. It is the only
// place that combines chat-session and task services; services themselves
// never import each other.
type Orchestrator struct {
	sessions domainchat.SessionService
	tasks    domaintask.TaskService
}

// NewOrchestrator creates an agent orchestrator wired to the chat session
// and task domain contracts.
func NewOrchestrator(sessions domainchat.SessionService, tasks domaintask.TaskService) *Orchestrator {
	return &Orchestrator{sessions: sessions, tasks: tasks}
}

// CreateAgentTaskRequest is the domain-level input for creating an async
// agent task.
type CreateAgentTaskRequest struct {
	Title      string                 `json:"title"`
	Model      string                 `json:"model"`
	Messages   []domainchat.Message   `json:"messages"`
	SkillChain []string               `json:"skill_chain"`
	Params     map[string]interface{} `json:"params"`
}

// CreateAgentTaskResponse is the domain-level result of creating an async
// agent task.
type CreateAgentTaskResponse struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Note      string `json:"note,omitempty"`
}

// CreateAgentTask creates a session and enqueues an async agent task via the
// task service. When no task service is configured (Redis unavailable), it
// returns a memory-fallback response so the caller can still answer the
// request without failing.
func (o *Orchestrator) CreateAgentTask(ctx context.Context, userID string, req CreateAgentTaskRequest) (*CreateAgentTaskResponse, error) {
	sess, err := o.sessions.Create(userID, "agent")
	if err != nil {
		return nil, fmt.Errorf("failed to create session")
	}

	if o.tasks != nil {
		taskType := "agent"
		skillChain := req.SkillChain
		if skillChain == nil {
			skillChain = []string{}
		}
		t, err := o.tasks.CreateTask(sess.ID, userID, taskType, skillChain, req.Params)
		if err != nil {
			return nil, fmt.Errorf("failed to create task")
		}
		return &CreateAgentTaskResponse{
			TaskID:    t.ID,
			SessionID: t.SessionID,
			Status:    string(t.Status),
		}, nil
	}

	// Fallback memory-based execution (no Redis available).
	return &CreateAgentTaskResponse{
		TaskID:    "task_memory_fallback",
		SessionID: sess.ID,
		Status:    "queued",
		Note:      "Redis not available — task will not be executed",
	}, nil
}
