// Package scheduler implements cron-based scheduled task execution.
// Scheduled triggers create AgentTasks via the TaskService and enqueue them
// to Redis Stream for worker execution.
package scheduler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Schedule represents a recurring job configuration linked to a persistent
// Task definition. When triggered, the scheduler creates a new TaskRun for
// the bound TaskID and enqueues it — it does NOT create a new Task each time.
type Schedule struct {
	ID           string                 `json:"id" bson:"_id"`
	TaskID       string                 `json:"task_id" bson:"task_id"` // links to persistent Task definition
	Name         string                 `json:"name" bson:"name"`
	ScheduleMode string                 `json:"-" bson:"-"` // "one_time" | "recurring"
	CronExpr     string                 `json:"cron_expr" bson:"cron_expr"`
	ScheduledAt  *time.Time             `json:"-" bson:"-"`
	Interval     time.Duration          `json:"-" bson:"interval_sec"`
	Enabled    bool                   `json:"enabled" bson:"enabled"`
	SkillChain []string               `json:"-" bson:"-"` // informational; stored on Task now
	Params     map[string]interface{} `json:"-" bson:"-"` // informational; stored on Task now
	ModelID    string                 `json:"-" bson:"-"`
	LastRun    *time.Time             `json:"last_run" bson:"last_run"`
	NextRun    time.Time              `json:"next_run" bson:"next_run"`
	CreatedAt  time.Time              `json:"created_at" bson:"created_at"`
}

// TaskCreator creates a new TaskRun (execution instance) for a task definition.
type TaskCreator interface {
	CreateRun(taskID string) (runID string, err error)
}

// Scheduler manages recurring task schedules.
type Scheduler struct {
	mu        sync.RWMutex
	schedules map[string]*Schedule
	creator   TaskCreator
	stopCh    chan struct{}
}

// New creates a new Scheduler.
func New(creator TaskCreator) *Scheduler {
	return &Scheduler{
		schedules: make(map[string]*Schedule),
		creator:   creator,
		stopCh:    make(chan struct{}),
	}
}

// Start begins the scheduler loop, checking for due jobs every 30 seconds.
func (s *Scheduler) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.runDueJobs(ctx)
			case <-s.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop gracefully stops the scheduler.
func (s *Scheduler) Stop() {
	close(s.stopCh)
}

// AddSchedule registers a new schedule.
func (s *Scheduler) AddSchedule(sch *Schedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sch.ID == "" {
		sch.ID = fmt.Sprintf("sch_%d", time.Now().UnixNano())
	}

	interval, err := parseCronExpr(sch.CronExpr)
	if err != nil {
		return fmt.Errorf("invalid cron expression %q: %w", sch.CronExpr, err)
	}
	sch.Interval = interval
	sch.CreatedAt = time.Now()
	sch.NextRun = time.Now().Add(interval)

	if sch.Params == nil {
		sch.Params = make(map[string]interface{})
	}

	s.schedules[sch.ID] = sch
	log.Printf("Scheduler: added schedule %q (interval: %v)", sch.Name, interval)
	return nil
}

// RemoveSchedule removes a schedule by ID.
func (s *Scheduler) RemoveSchedule(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.schedules, id)
}

// GetSchedule returns a schedule by ID.
func (s *Scheduler) GetSchedule(id string) (*Schedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sch, ok := s.schedules[id]
	if !ok {
		return nil, fmt.Errorf("schedule %q not found", id)
	}
	return sch, nil
}

// ListSchedules returns all registered schedules.
func (s *Scheduler) ListSchedules() []*Schedule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Schedule, 0, len(s.schedules))
	for _, sch := range s.schedules {
		result = append(result, sch)
	}
	return result
}

// runDueJobs checks all schedules and runs those that are due.
func (s *Scheduler) runDueJobs(ctx context.Context) {
	s.mu.RLock()
	var due []*Schedule
	now := time.Now()
	for _, sch := range s.schedules {
		if sch.Enabled && !now.Before(sch.NextRun) {
			due = append(due, sch)
		}
	}
	s.mu.RUnlock()

	for _, sch := range due {
		s.executeJob(ctx, sch)
	}
}

// executeJob creates a new TaskRun for the schedule's bound Task definition
// and enqueues it. The schedule links to a persistent Task via TaskID.
func (s *Scheduler) executeJob(ctx context.Context, sch *Schedule) {
	now := time.Now()
	log.Printf("Scheduler: executing %q (task_id=%s)", sch.Name, sch.TaskID)

	_, err := s.creator.CreateRun(sch.TaskID)
	if err != nil {
		log.Printf("Scheduler: failed to create run for %q (task_id=%s): %v", sch.Name, sch.TaskID, err)
		return
	}

	lastRun := now
	sch.LastRun = &lastRun

	if sch.Interval == 0 {
		// One-time schedule: disable after first execution.
		sch.Enabled = false
		log.Printf("Scheduler: one-time schedule %q completed, disabled", sch.Name)
	} else {
		sch.NextRun = now.Add(sch.Interval)
		log.Printf("Scheduler: completed %q, next run at %s", sch.Name, sch.NextRun.Format(time.RFC3339))
	}
}

// parseCronExpr parses a cron expression into an interval (approximate)
// and returns the next occurrence time from now.
func parseCronExpr(expr string) (time.Duration, error) {
	// Standard 5-part cron: minute hour day-of-month month day-of-week
	if len(expr) > 0 && expr[0] >= '0' && expr[0] <= '9' && strings.Count(expr, " ") >= 4 {
		dur, err := nextCronDuration(expr)
		if err == nil {
			return dur, nil
		}
	}
	switch expr {
	case "every_1m":
		return 1 * time.Minute, nil
	case "every_5m":
		return 5 * time.Minute, nil
	case "every_15m":
		return 15 * time.Minute, nil
	case "every_30m":
		return 30 * time.Minute, nil
	case "every_1h":
		return 1 * time.Hour, nil
	case "every_6h":
		return 6 * time.Hour, nil
	case "every_12h":
		return 12 * time.Hour, nil
	case "every_24h":
		return 24 * time.Hour, nil
	case "daily_09:00":
		return time.Duration(getNextHoursUntil(9)) * time.Hour, nil
	case "weekly_monday_09:00":
		return 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported cron expression %q (supported: every_1m, every_5m, every_1h, every_24h, daily_09:00, weekly_monday_09:00)", expr)
	}
}


// nextCronTime returns the next occurrence time for a standard 5-part cron expression.
func nextCronTime(expr string) (time.Time, error) {
	// Parse: minute hour day month weekday
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return time.Time{}, fmt.Errorf("invalid cron: want 5 parts, got %d", len(parts))
	}
	minute, _ := strconv.Atoi(parts[0])
	hour, _ := strconv.Atoi(parts[1])
	now := time.Now()
	candidate := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !candidate.After(now) {
		candidate = candidate.Add(24 * time.Hour)
	}
	return candidate, nil
}

// nextCronDuration calculates interval until next cron occurrence (approximate).
func nextCronDuration(expr string) (time.Duration, error) {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return 0, fmt.Errorf("invalid cron: want 5 parts, got %d", len(parts))
	}
	minute, _ := strconv.Atoi(parts[0])
	hour, _ := strconv.Atoi(parts[1])
	day := parts[2]
	month := parts[3]
	weekday := parts[4]

	// Estimate: daily, weekly, monthly, yearly
	if day == "*" && month == "*" && weekday == "*" {
		// Daily
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		return next.Sub(now), nil
	}
	if day == "*" && month == "*" && weekday != "*" {
		// Specific weekday
		w, _ := strconv.Atoi(weekday) // 0=Sun, 1=Mon, ...
		if w >= 0 && w <= 6 {
			now := time.Now()
			daysUntil := (w - int(now.Weekday()) + 7) % 7
			if daysUntil == 0 {
				candidate := time.Date(now.Year(), now.Month(), now.Day()+daysUntil, hour, minute, 0, 0, now.Location())
				if !candidate.After(now) {
					daysUntil = 7
				}
			}
			next := time.Date(now.Year(), now.Month(), now.Day()+daysUntil, hour, minute, 0, 0, now.Location())
			return next.Sub(now), nil
		}
	}
	// Default: daily at specified time
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next.Sub(now), nil
}

// getNextHoursUntil is kept for backward compatibility.

func getNextHoursUntil(targetHour int) int {
	now := time.Now()
	currentHour := now.Hour()
	if currentHour < targetHour {
		return targetHour - currentHour
	}
	return (24 - currentHour) + targetHour
}

// ScheduleProvider reads scheduled task definitions from persistent storage.
type ScheduleProvider interface {
	ListScheduled(ctx context.Context, skip, limit int64) (tasks []TaskDef, total int64, err error)
	MarkScheduledDone(ctx context.Context, id string) error
}

// TaskDef is a minimal scheduled task definition needed by the scheduler.
type TaskDef struct {
	ID           string
	UserID       string
	Title        string
	ScheduleMode string
	CronExpr     string
	ScheduledAt  *time.Time
	SkillChain   []string
	Params       map[string]interface{}
	ModelID      string
}

// LoadFromDB paginates through all scheduled tasks from the provider and registers them.
func (s *Scheduler) LoadFromDB(ctx context.Context, provider ScheduleProvider) (int, error) {
	var loaded int
	var skip int64
	const batchSize int64 = 100
	for {
		tasks, total, err := provider.ListScheduled(ctx, skip, batchSize)
		if err != nil {
			return loaded, err
		}
		for _, t := range tasks {
			sch := &Schedule{
				TaskID:       t.ID,
				Name:         t.Title,
				ScheduleMode: t.ScheduleMode,
				CronExpr:     t.CronExpr,
				ScheduledAt:  t.ScheduledAt,
				Enabled:      true,
				SkillChain:   t.SkillChain,
				Params:       t.Params,
				ModelID:      t.ModelID,
				CreatedAt:    time.Now(),
			}
			switch t.ScheduleMode {
			case "recurring":
				if t.CronExpr == "" {
					log.Printf("Scheduler: skipping task %s (%s): recurring mode but no cron_expr", t.ID, t.Title)
					continue
				}
				interval, err := parseCronExpr(t.CronExpr)
				if err != nil {
					log.Printf("Scheduler: skipping task %s (%s): invalid cron %q: %v", t.ID, t.Title, t.CronExpr, err)
					continue
				}
				sch.Interval = interval
				sch.NextRun = time.Now().Add(interval)
			case "one_time":
				if t.ScheduledAt == nil {
					log.Printf("Scheduler: skipping task %s (%s): one_time mode but no scheduled_at", t.ID, t.Title)
					continue
				}
				sch.NextRun = *t.ScheduledAt
				sch.Interval = 0 // one-shot, no recurrence
			default:
				log.Printf("Scheduler: skipping task %s (%s): unknown mode %q", t.ID, t.Title, t.ScheduleMode)
				continue
			}
			s.AddSchedule(sch)
			loaded++
		}
		skip += batchSize
		if skip >= total {
			break
		}
	}
	log.Printf("Scheduler: loaded %d scheduled tasks from DB", loaded)
	return loaded, nil
}
