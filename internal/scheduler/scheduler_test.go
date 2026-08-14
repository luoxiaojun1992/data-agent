package scheduler

import (
	"context"
	"testing"
	"time"
)

// mockProvider implements ScheduleProvider for tests.
type mockProvider struct {
	tasks []TaskDef
}

func (m *mockProvider) ListScheduled(ctx context.Context, skip, limit int64, now time.Time) ([]TaskDef, int64, error) {
	return m.tasks, int64(len(m.tasks)), nil
}

func (m *mockProvider) MarkScheduledDone(ctx context.Context, id string) error {
	return nil
}

func TestReloadFromDB_SetsEnabled(t *testing.T) {
	now := time.Now()
	at := now.Add(-1 * time.Minute) // already due
	s := New(&fakeCreator{})
	s.SetProvider(&mockProvider{tasks: []TaskDef{
		{
			ID:           "task_one",
			Title:        "one-shot",
			ScheduleMode: "one_time",
			ScheduledAt:  &at,
		},
	}})

	s.reloadFromDB(context.Background())

	s.mu.RLock()
	sch, ok := s.schedules["task_one"]
	s.mu.RUnlock()
	if !ok {
		t.Fatal("expected task_one to be loaded")
	}
	if !sch.Enabled {
		t.Fatalf("reloadFromDB loaded task with Enabled=false; one-shot tasks must be Enabled=true to run")
	}
	if sch.Interval != 0 {
		t.Errorf("one-shot Interval = %v, want 0", sch.Interval)
	}
}

func TestReloadFromDB_RecurringSetsEnabled(t *testing.T) {
	s := New(&fakeCreator{})
	s.SetProvider(&mockProvider{tasks: []TaskDef{
		{
			ID:           "task_rec",
			Title:        "recurring",
			ScheduleMode: "recurring",
			CronExpr:     "0 * * * *",
		},
	}})

	s.reloadFromDB(context.Background())

	s.mu.RLock()
	sch, ok := s.schedules["task_rec"]
	s.mu.RUnlock()
	if !ok {
		t.Fatal("expected task_rec to be loaded")
	}
	if !sch.Enabled {
		t.Fatalf("reloadFromDB loaded recurring task with Enabled=false")
	}
}

type fakeCreator struct{}

func (f *fakeCreator) CreateRun(taskID string) (string, error) {
	return "run_" + taskID, nil
}
