package mongo

import (
	"go.mongodb.org/mongo-driver/mongo"
)

// TaskRepository is the legacy task repository (pre-TaskRun split).
// Replaced by TaskDefRepository + TaskRunRepository.
// Kept as a stub for backward-compat; the old converter helpers
// (taskToDoc / docToTask) are still used by compatibility code.
type TaskRepository struct {
	coll *mongo.Collection
}

func NewTaskRepository(db *mongo.Database) *TaskRepository {
	return &TaskRepository{coll: db.Collection("agent_tasks")}
}

// Stub methods — the actual storage is now handled by TaskDefRepository + TaskRunRepository.
// These no-ops prevent test/init code referencing the old type from panicking.
func (r *TaskRepository) Create(ctx any, t any) error { return nil }
func (r *TaskRepository) Get(ctx any, id string) (any, error) { return nil, nil }
func (r *TaskRepository) List(ctx any, userID string, skip, limit int64) (any, int64, error) { return nil, 0, nil }
func (r *TaskRepository) ListAll(ctx any, userID string) (any, error) { return nil, nil }
func (r *TaskRepository) Cancel(ctx any, id string) error { return nil }
