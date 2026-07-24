package task

import (
	"time"

	"github.com/google/uuid"
)

// Status represents the task lifecycle state.
type Status string

const (
	StatusPending   Status = "pending"
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusRetrying  Status = "retrying"
	StatusCancelled Status = "cancelled"
)

// Task represents an async agent task (MongoDB).
type Task struct {
	ID          string                 `json:"task_id"`
	SessionID   string                 `json:"session_id"`
	UserID      string                 `json:"user_id"`
	Type        string                 `json:"type"` // "agent_exec", "scheduled_exec"
	ModelID     string                 `json:"model_id"` // bound model ID (ModelEntry.ID); worker uses this to select a Runtime
	Status      Status                 `json:"status"`
	SkillChain  []string               `json:"skill_chain"`
	Params      map[string]interface{} `json:"params"`
	Result      map[string]interface{} `json:"result,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Progress    TaskProgress           `json:"progress"`
	RetryCount  int                    `json:"retry_count"`
	MaxRetries  int                    `json:"max_retries"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	DurationMs  int64                  `json:"duration_ms"`
}

// TaskProgress tracks execution progress.
type TaskProgress struct {
	CurrentStep int    `json:"current_step"`
	TotalSteps  int    `json:"total_steps"`
	Message     string `json:"message"`
	Percent     int    `json:"percent"`
}

// NewTask creates a new task with a generated ID.
func NewTask(sessionID, userID, taskType string, skillChain []string, params map[string]interface{}, modelID string) *Task {
	now := time.Now()
	totalSteps := len(skillChain)
	if totalSteps == 0 {
		totalSteps = 1
	}
	return &Task{
		ID:         "task_" + uuid.New().String(),
		SessionID:  sessionID,
		UserID:     userID,
		Type:       taskType,
		ModelID:    modelID,
		Status:     StatusPending,
		SkillChain: skillChain,
		Params:     params,
		Progress: TaskProgress{
			CurrentStep: 0,
			TotalSteps:  totalSteps,
			Message:     "Task created",
			Percent:     0,
		},
		MaxRetries: 3,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// ScheduledTask represents a cron-scheduled task (MongoDB).
type ScheduledTask struct {
	ID         string                 `json:"scheduled_task_id"`
	UserID     string                 `json:"user_id"`
	Name       string                 `json:"name"`
	CronExpr   string                 `json:"cron_expr"`
	SkillChain []string               `json:"skill_chain"`
	Params     map[string]interface{} `json:"params"`
	ModelID    string                 `json:"model_id"` // bound model for scheduled runs
	Status     string                 `json:"status"` // active, paused, deleted
	LastRunAt  *time.Time             `json:"last_run_at,omitempty"`
	NextRunAt  *time.Time             `json:"next_run_at,omitempty"`
	FailCount  int                    `json:"fail_count"`
	CreatedAt  time.Time              `json:"created_at"`
}

// QueueMessage is the JSON message format for Redis Stream.
type QueueMessage struct {
	TaskID     string                 `json:"task_id"`
	SessionID  string                 `json:"session_id"`
	UserID     string                 `json:"user_id"`
	Type       string                 `json:"type"`
	ModelID    string                 `json:"model_id"` // worker selects Runtime by this
	SkillChain []string               `json:"skill_chain"`
	Params     map[string]interface{} `json:"params"`
	CreatedAt  string                 `json:"created_at"`
}
