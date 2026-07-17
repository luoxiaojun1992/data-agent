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
	ID          string                 `bson:"_id" json:"task_id"`
	SessionID   string                 `bson:"session_id" json:"session_id"`
	UserID      string                 `bson:"user_id" json:"user_id"`
	Type        string                 `bson:"type" json:"type"` // "agent_exec", "scheduled_exec"
	Status      Status                 `bson:"status" json:"status"`
	SkillChain  []string               `bson:"skill_chain" json:"skill_chain"`
	Params      map[string]interface{} `bson:"params" json:"params"`
	Result      map[string]interface{} `bson:"result,omitempty" json:"result,omitempty"`
	Error       string                 `bson:"error,omitempty" json:"error,omitempty"`
	Progress    TaskProgress           `bson:"progress" json:"progress"`
	RetryCount  int                    `bson:"retry_count" json:"retry_count"`
	MaxRetries  int                    `bson:"max_retries" json:"max_retries"`
	CreatedAt   time.Time              `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time              `bson:"updated_at" json:"updated_at"`
	CompletedAt *time.Time             `bson:"completed_at,omitempty" json:"completed_at,omitempty"`
	DurationMs  int64                  `bson:"duration_ms" json:"duration_ms"`
}

// TaskProgress tracks execution progress.
type TaskProgress struct {
	CurrentStep int    `bson:"current_step" json:"current_step"`
	TotalSteps  int    `bson:"total_steps" json:"total_steps"`
	Message     string `bson:"message" json:"message"`
	Percent     int    `bson:"percent" json:"percent"`
}

// NewTask creates a new task with a generated ID.
func NewTask(sessionID, userID, taskType string, skillChain []string, params map[string]interface{}) *Task {
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
	ID         string                 `bson:"_id" json:"scheduled_task_id"`
	UserID     string                 `bson:"user_id" json:"user_id"`
	Name       string                 `bson:"name" json:"name"`
	CronExpr   string                 `bson:"cron_expr" json:"cron_expr"`
	SkillChain []string               `bson:"skill_chain" json:"skill_chain"`
	Params     map[string]interface{} `bson:"params" json:"params"`
	Status     string                 `bson:"status" json:"status"` // active, paused, deleted
	LastRunAt  *time.Time             `bson:"last_run_at,omitempty" json:"last_run_at,omitempty"`
	NextRunAt  *time.Time             `bson:"next_run_at,omitempty" json:"next_run_at,omitempty"`
	FailCount  int                    `bson:"fail_count" json:"fail_count"`
	CreatedAt  time.Time              `bson:"created_at" json:"created_at"`
}

// QueueMessage is the JSON message format for Redis Stream.
type QueueMessage struct {
	TaskID     string                 `json:"task_id"`
	SessionID  string                 `json:"session_id"`
	UserID     string                 `json:"user_id"`
	Type       string                 `json:"type"`
	SkillChain []string               `json:"skill_chain"`
	Params     map[string]interface{} `json:"params"`
	CreatedAt  string                 `json:"created_at"`
}
