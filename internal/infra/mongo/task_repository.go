package mongo

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type TaskRepository struct {
	coll *mongo.Collection
}

func NewTaskRepository(db *mongo.Database) *TaskRepository {
	return &TaskRepository{coll: db.Collection("agent_tasks")}
}

func (r *TaskRepository) Create(ctx context.Context, t *task.Task) error {
	_, err := r.coll.InsertOne(ctx, taskToDoc(t))
	return err
}

func (r *TaskRepository) Get(ctx context.Context, id string) (*task.Task, error) {
	var d bson.M
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&d)
	if err != nil {
		return nil, err
	}
	return docToTask(d), nil
}

func (r *TaskRepository) Cancel(ctx context.Context, id string) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"status": task.StatusCancelled}})
	return err
}

func (r *TaskRepository) List(ctx context.Context, userID string, status string, skip, limit int64) ([]*task.Task, int64, error) {
	filter := bson.M{"user_id": userID}
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
	tasks := make([]*task.Task, len(docs))
	for i, d := range docs {
		tasks[i] = docToTask(d)
	}
	return tasks, total, nil
}

func (r *TaskRepository) ListAll(ctx context.Context, userID string) ([]*task.Task, error) {
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
		tasks[i] = docToTask(d)
	}
	return tasks, nil
}

func (r *TaskRepository) UpdateProgress(ctx context.Context, id string, p *task.TaskProgress) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"progress": taskProgressToDoc(*p)}})
	return err
}

func (r *TaskRepository) UpdateResult(ctx context.Context, id string, result map[string]interface{}) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"result": result, "status": task.StatusCompleted}})
	return err
}

func (r *TaskRepository) Retry(ctx context.Context, id string, t *task.Task) error {
	_, err := r.coll.ReplaceOne(ctx, bson.M{"_id": id}, taskToDoc(t))
	return err
}

func (r *TaskRepository) CountByStatus(ctx context.Context, userID string, status string) (int64, error) {
	return r.coll.CountDocuments(ctx, bson.M{"user_id": userID, "status": status})
}

var _ repository.TaskRepository = (*TaskRepository)(nil)
