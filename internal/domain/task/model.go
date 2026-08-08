package task

import (
	"encoding/json"
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

// Task types.
const (
	TaskTypeAgentExec     = "agent_exec"
	TaskTypeScheduledExec = "scheduled_exec"
	TaskTypeKBIndex       = "kb_index"
)

// Schedule modes.
const (
	ScheduleModeOneTime  = "one_time"  // execute once at specific time
	ScheduleModeRecurring = "recurring" // execute on cron schedule
)

// Task represents an async agent task definition(MongoDB).
// Task definitions are persistent and do NOT carry execution state.
// Execution state lives on TaskRun (run-level); Task only records the
// identifier of the last execution via LastRunID.
type Task struct {
	ID          string                 `json:"task_id"`
	UserID      string                 `json:"user_id"`
	Title       string                 `json:"title"`  // human-readable label
	Description string                 `json:"description,omitempty"`
	Type        string                 `json:"type"`   // "agent_exec", "scheduled_exec", "kb_index"
	ModelID     string                 `json:"model_id"`
	SkillChain  []string               `json:"skill_chain"`
	Params      map[string]interface{} `json:"params"`
	CronExpr         string     `json:"cron_expr,omitempty" bson:"cron_expr,omitempty"`
	ScheduleMode     string     `json:"schedule_mode,omitempty" bson:"schedule_mode,omitempty"`
	ScheduledAt      *time.Time `json:"scheduled_at,omitempty" bson:"scheduled_at,omitempty"`
	RunCount         int64                  `json:"run_count" bson:"run_count"`           // incremented atomically per run creation
	LastRunAt   *time.Time             `json:"last_run_at,omitempty" bson:"last_run_at,omitempty"`
	ScheduledEnabled bool              `json:"scheduled_enabled" bson:"scheduled_enabled"` // on/off switch for scheduled tasks (non-scheduled always true)
	ScheduledDone    bool              `json:"scheduled_done" bson:"scheduled_done"`       // only set true for one-shot scheduled tasks
	CreatedAt   time.Time              `json:"created_at" bson:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" bson:"updated_at"`
}

// TaskProgress tracks execution progress.
type TaskProgress struct {
	CurrentStep int    `json:"current_step"`
	TotalSteps  int    `json:"total_steps"`
	Message     string `json:"message"`
	Percent     int    `json:"percent"`
}

// NewTask creates a new task definition with a generated ID.
func NewTask(userID, taskType string, skillChain []string, params map[string]interface{}, modelID string) *Task {
	now := time.Now()
	title := ""
	if params != nil {
		if v, ok := params["title"].(string); ok {
			title = v
		}
	}
	return &Task{
		ID:         "task_" + uuid.New().String(),
		UserID:     userID,
		Title:      title,
		Type:       taskType,
		ModelID:    modelID,
		SkillChain: skillChain,
		Params:     params,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// TaskRun represents one execution of a Task (MongoDB).
type TaskRun struct {
	ID          string                 `json:"run_id"`
	TaskID      string                 `json:"task_id"`
	UserID      string                 `json:"user_id"`
	Type        string                 `json:"type"`
	ModelID     string                 `json:"model_id"`
	SessionID   string                 `json:"session_id"`
	Status      Status                 `json:"status"`
	SkillChain  []string               `json:"skill_chain"`
	Params      map[string]interface{} `json:"params"`
	Result      map[string]interface{} `json:"result,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Progress    TaskProgress           `json:"progress"`
	RetryCount  int                    `json:"retry_count"`
	MaxRetries  int                    `json:"max_retries"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	DurationMs  int64                  `json:"duration_ms"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// NewTaskRun creates a new execution run for a task definition.
func NewTaskRun(task *Task) *TaskRun {
	now := time.Now()
	totalSteps := len(task.SkillChain)
	if totalSteps == 0 {
		totalSteps = 1
	}
	return &TaskRun{
		ID:         "run_" + uuid.New().String(),
		TaskID:     task.ID,
		UserID:     task.UserID,
		Type:       task.Type,
		ModelID:    task.ModelID,
		SkillChain: task.SkillChain,
		Params:     task.Params,
		Status:     StatusPending,
		Progress: TaskProgress{
			CurrentStep: 0,
			TotalSteps:  totalSteps,
			Message:     "Run created",
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

// QueueMessage is the unified JSON envelope for all Redis Stream messages.
// The Type field determines the Payload schema:
//   - "agent_task" → AgentTaskPayload (task_run record reference)
//   - "kb_index"   → KBIndexPayload
type QueueMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// AgentTaskPayload references an existing TaskRun record for agent execution.
type AgentTaskPayload struct {
	RunID      string   `json:"run_id"`
	TaskID     string   `json:"task_id"`
	SessionID  string   `json:"session_id"`
	UserID     string   `json:"user_id"`
	ModelID    string   `json:"model_id"`
	SkillChain []string `json:"skill_chain"`
	CreatedAt  string   `json:"created_at"`
}

// KBIndexPayload is the payload for KB indexing jobs.
type KBIndexPayload struct {
	DocID        string `json:"doc_id"`
	GridFSFileID string `json:"gridfs_file_id"`
}
