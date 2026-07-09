// Package scheduler implements cron-based scheduled task execution.
// Scheduled triggers create AgentTasks via the TaskService and enqueue them
// to Redis Stream for worker execution.
package scheduler

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Schedule represents a recurring job configuration.
type Schedule struct {
	ID        string                 `json:"id" bson:"_id"`
	Name      string                 `json:"name" bson:"name"`
	CronExpr  string                 `json:"cron_expr" bson:"cron_expr"`   // simplified: "every_5m", "every_1h", "daily_09:00"
	Interval  time.Duration          `json:"-" bson:"interval_sec"`         // parsed interval in seconds
	Enabled   bool                   `json:"enabled" bson:"enabled"`
	SkillChain []string              `json:"skill_chain" bson:"skill_chain"`
	Params    map[string]interface{} `json:"params" bson:"params"`
	LastRun   *time.Time             `json:"last_run" bson:"last_run"`
	NextRun   time.Time              `json:"next_run" bson:"next_run"`
	CreatedAt time.Time              `json:"created_at" bson:"created_at"`
}

// TaskCreator is the interface Scheduler uses to create AgentTasks.
type TaskCreator interface {
	CreateTask(sessionID, userID, taskType string, skillChain []string, params map[string]interface{}) (taskID string, err error)
}

// Scheduler manages recurring task schedules.
type Scheduler struct {
	mu       sync.RWMutex
	schedules map[string]*Schedule
	creator  TaskCreator
	stopCh   chan struct{}
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

// executeJob runs a single scheduled job.
func (s *Scheduler) executeJob(ctx context.Context, sch *Schedule) {
	now := time.Now()
	log.Printf("Scheduler: executing %q", sch.Name)

	sessionID := fmt.Sprintf("sched_%s_%d", sch.ID, now.Unix())
	_, err := s.creator.CreateTask(sessionID, "system", "scheduled", sch.SkillChain, sch.Params)
	if err != nil {
		log.Printf("Scheduler: failed to create task for %q: %v", sch.Name, err)
		return
	}

	lastRun := now
	sch.LastRun = &lastRun
	sch.NextRun = now.Add(sch.Interval)
	log.Printf("Scheduler: completed %q, next run at %s", sch.Name, sch.NextRun.Format(time.RFC3339))
}

// parseCronExpr parses simplified cron expressions into time.Duration.
func parseCronExpr(expr string) (time.Duration, error) {
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

func getNextHoursUntil(targetHour int) int {
	now := time.Now()
	currentHour := now.Hour()
	if currentHour < targetHour {
		return targetHour - currentHour
	}
	return (24 - currentHour) + targetHour
}
