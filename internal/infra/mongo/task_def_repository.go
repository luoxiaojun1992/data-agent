package mongo

import (
	"context"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TaskDefRepository stores task definitions in the agent_task_defs collection.
type TaskDefRepository struct {
	coll *mongo.Collection
}

func NewTaskDefRepository(db *mongo.Database) *TaskDefRepository {
	return &TaskDefRepository{coll: db.Collection("agent_task_defs")}
}

func (r *TaskDefRepository) Create(ctx context.Context, t *task.Task) error {
	_, err := r.coll.InsertOne(ctx, taskDefToDoc(t))
	return err
}

func (r *TaskDefRepository) Get(ctx context.Context, id string) (*task.Task, error) {
	var d bson.M
	if err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&d); err != nil {
		return nil, err
	}
	return docToTaskDef(d), nil
}

func (r *TaskDefRepository) UpdateLastRun(ctx context.Context, id string, runAt time.Time) error {
	// Atomic single op: bump run_count + set last_run_at + updated_at.
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$inc": bson.M{"run_count": int64(1)},
		"$set": bson.M{"last_run_at": runAt, "updated_at": runAt},
	})
	return err
}

func (r *TaskDefRepository) Cancel(ctx context.Context, id string) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *TaskDefRepository) List(ctx context.Context, userID string, skip, limit int64) ([]*task.Task, int64, error) {
	filter := bson.M{"user_id": userID}
	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	opts := options.Find().SetSort(bson.M{"created_at": -1}).SetSkip(skip).SetLimit(limit)
	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, 0, err
	}
	tasks := make([]*task.Task, len(docs))
	for i, d := range docs {
		tasks[i] = docToTaskDef(d)
	}
	return tasks, total, nil
}

func (r *TaskDefRepository) ListAll(ctx context.Context, userID string) ([]*task.Task, error) {
	cursor, err := r.coll.Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	tasks := make([]*task.Task, len(docs))
	for i, d := range docs {
		tasks[i] = docToTaskDef(d)
	}
	return tasks, nil
}

// ---- TaskRun Repository ----

type TaskRunRepository struct {
	coll *mongo.Collection
}

func NewTaskRunRepository(db *mongo.Database) *TaskRunRepository {
	return &TaskRunRepository{coll: db.Collection("agent_task_runs")}
}

func (r *TaskRunRepository) Create(ctx context.Context, tr *task.TaskRun) error {
	_, err := r.coll.InsertOne(ctx, taskRunToDoc(tr))
	return err
}

func (r *TaskRunRepository) Get(ctx context.Context, id string) (*task.TaskRun, error) {
	var d bson.M
	if err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&d); err != nil {
		return nil, err
	}
	return docToTaskRun(d), nil
}

func (r *TaskRunRepository) List(ctx context.Context, taskID string, status string, skip, limit int64) ([]*task.TaskRun, int64, error) {
	filter := bson.M{"task_id": taskID}
	if status != "" {
		filter["status"] = status
	}
	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	opts := options.Find().SetSort(bson.M{"created_at": -1}).SetSkip(skip).SetLimit(limit)
	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, 0, err
	}
	runs := make([]*task.TaskRun, len(docs))
	for i, d := range docs {
		runs[i] = docToTaskRun(d)
	}
	return runs, total, nil
}

func (r *TaskRunRepository) UpdateStatus(ctx context.Context, id string, status task.Status) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"status": status}})
	return err
}

func (r *TaskRunRepository) UpdateResult(ctx context.Context, id string, result map[string]interface{}) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id},
		bson.M{"$set": bson.M{"result": result, "status": task.StatusCompleted, "completed_at": time.Now()}})
	return err
}

func (r *TaskRunRepository) UpdateError(ctx context.Context, id string, errMsg string) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id},
		bson.M{"$set": bson.M{"error": errMsg, "status": task.StatusFailed, "completed_at": time.Now()}})
	return err
}

func (r *TaskRunRepository) UpdateSessionID(ctx context.Context, id, sessionID string) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"session_id": sessionID}})
	return err
}

func (r *TaskRunRepository) Cancel(ctx context.Context, id string) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"status": task.StatusCancelled}})
	return err
}
