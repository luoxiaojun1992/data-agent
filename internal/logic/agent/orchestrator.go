// Package agent contains the orchestration layer for agent use cases.
// The orchestrator combines multiple services (chat session, task) to
// fulfill use cases that span service boundaries, eliminating same-layer
// service dependencies. It depends on domain contracts, not on concrete
// service implementations.
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
)

// Orchestrator coordinates cross-service agent use cases. It is the only
// place that combines chat-session and task services; services themselves
// never import each other.
type Orchestrator struct {
	sessions domainchat.SessionService
	tasks    domaintask.TaskService
	provider *modelcfg.Provider // resolves the default model when req.Model is empty
}

// NewOrchestrator creates an agent orchestrator wired to the chat session
// and task domain contracts. provider may be nil (default model disabled).
func NewOrchestrator(sessions domainchat.SessionService, tasks domaintask.TaskService, provider *modelcfg.Provider) *Orchestrator {
	return &Orchestrator{sessions: sessions, tasks: tasks, provider: provider}
}

// CreateAgentTaskRequest is the domain-level input for creating an async
// agent task.
type CreateAgentTaskRequest struct {
	Title      string                 `json:"title"`
	Model      string                 `json:"model"` // ModelEntry.ID; empty = default
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

// resolveModel returns the model ID to bind: req.Model when set, otherwise
// the default LLM model ID (empty when no default is configured).
func (o *Orchestrator) resolveModel(ctx context.Context, modelID string) string {
	if modelID != "" {
		return modelID
	}
	if o.provider == nil {
		return ""
	}
	if dm, err := o.provider.DefaultModel(ctx); err == nil && dm != nil {
		return dm.ID
	}
	return ""
}

// CreateAgentTask creates a session (binding the model) and enqueues an async
// agent task via the task service. When no task service is configured (Redis
// unavailable), it returns a memory-fallback response so the caller can still
// answer the request without failing.
//
// SPEC-062: The model is resolved from req.Model (empty → default) and bound
// to the session + task so the worker (SPEC-063) can select the right Runtime.
func (o *Orchestrator) CreateAgentTask(ctx context.Context, userID string, req CreateAgentTaskRequest) (*CreateAgentTaskResponse, error) {
	modelID := o.resolveModel(ctx, req.Model)
	sess, err := o.sessions.Create(userID, "agent", modelID)
	if err != nil {
		return nil, fmt.Errorf("failed to create session")
	}

	if o.tasks != nil {
		taskType := "agent"
		skillChain := req.SkillChain
		if skillChain == nil {
			skillChain = []string{}
		}
		// SPEC-063: embed the title + last user message into Params so the
		// async executor (deriveUserMessage) can recover the user input that
		// would otherwise be lost at the task boundary.
		params := enrichTaskParams(req)
		t, err := o.tasks.CreateTask(sess.ID, userID, taskType, skillChain, params, sess.ModelID)
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

// enrichTaskParams merges caller-provided params with the request title and
// last user message so the async AgentExecutor (SPEC-063 deriveUserMessage)
// can recover the user input. Caller-provided keys always take precedence and
// are never overwritten.
func enrichTaskParams(req CreateAgentTaskRequest) map[string]interface{} {
	params := map[string]interface{}{}
	for k, v := range req.Params {
		params[k] = v
	}
	if req.Title != "" {
		if _, ok := params["title"]; !ok {
			params["title"] = req.Title
		}
	}
	// Inject the last user message only when the caller hasn't already supplied
	// one of the conventional message keys (query/message/prompt/description).
	if !hasUserMessageKey(params) {
		if msg := lastUserMessageText(req.Messages); msg != "" {
			params["message"] = msg
		}
	}
	return params
}

// hasUserMessageKey reports whether params already carry a user-message key
// that deriveUserMessage would pick up.
func hasUserMessageKey(params map[string]interface{}) bool {
	for _, k := range []string{"query", "message", "prompt", "description"} {
		if _, ok := params[k]; ok {
			return true
		}
	}
	return false
}

// lastUserMessageText returns the content of the last user-role message, or
// "" when there is none.
func lastUserMessageText(msgs []domainchat.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" && strings.TrimSpace(msgs[i].Content) != "" {
			return msgs[i].Content
		}
	}
	return ""
}
