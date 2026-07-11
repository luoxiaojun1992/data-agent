package task

import (
	"context"
	"fmt"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/luoxiaojun1992/data-agent/internal/queue"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const collTasks = "agent_tasks"

// Service handles task lifecycle operations.
type Service struct {
	coll   *mongo.Collection
	stream *queue.Stream
}

// NewService creates a task service.
func NewService(db *mongo.Database, stream *queue.Stream) *Service {
	return &Service{
		coll:   db.Collection(collTasks),
		stream: stream,
	}
}

// CreateTask creates a new task, persists it, and enqueues it.
func (s *Service) CreateTask(sessionID, userID, taskType string, skillChain []string, params map[string]interface{}, cronExpr string) (*task.Task, error) {
	t := task.NewTask(sessionID, userID, taskType, skillChain, params)
	t.Status = task.StatusQueued

	// Store cron expression
	if cronExpr != "" {
		if t.Params == nil {
			t.Params = make(map[string]interface{})
		}
		t.Params["cron_expr"] = cronExpr
	}

	// Persist to MongoDB
	_, err := s.coll.InsertOne(context.Background(), t)
	if err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}

	// Enqueue to Redis Stream
	if err := s.stream.Enqueue(context.Background(), t); err != nil {
		return nil, fmt.Errorf("enqueue task: %w", err)
	}

	return t, nil
}

// GetTask retrieves a task by ID.
func (s *Service) GetTask(taskID string) (*task.Task, error) {
	var t task.Task
	err := s.coll.FindOne(context.Background(), bson.M{"_id": taskID}).Decode(&t)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("task %q not found", taskID)
		}
		return nil, fmt.Errorf("find task: %w", err)
	}
	return &t, nil
}

// CancelTask cancels a running or queued task.
func (s *Service) CancelTask(taskID string) error {
	_, err := s.coll.UpdateOne(context.Background(),
		bson.M{"_id": taskID, "status": bson.M{"$in": []string{"queued", "running"}}},
		bson.M{"$set": bson.M{"status": task.StatusCancelled, "updated_at": time.Now()}},
	)
	if err != nil {
		return fmt.Errorf("cancel task: %w", err)
	}
	return nil
}

// ListTasks returns tasks for a user.
func (s *Service) ListTasks(userID string) ([]task.Task, error) {
	opts := options.Find().SetSort(bson.M{"created_at": -1}).SetLimit(50)
	cursor, err := s.coll.Find(context.Background(), bson.M{"user_id": userID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	var tasks []task.Task
	if err := cursor.All(context.Background(), &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

// UpdateTaskProgress updates the progress of a running task.
func (s *Service) UpdateTaskProgress(taskID string, progress task.TaskProgress) error {
	_, err := s.coll.UpdateOne(context.Background(),
		bson.M{"_id": taskID},
		bson.M{"$set": bson.M{"progress": progress, "updated_at": time.Now()}},
	)
	return err
}

// UpdateTaskResult updates the task with completion result.
func (s *Service) UpdateTaskResult(taskID string, status task.Status, result map[string]interface{}, errMsg string, durationMs int64) error {
	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"status":       status,
			"result":       result,
			"error":        errMsg,
			"duration_ms":  durationMs,
			"completed_at": now,
			"updated_at":   now,
		},
	}
	_, err := s.coll.UpdateOne(context.Background(), bson.M{"_id": taskID}, update)
	return err
}

// UpdateStatus updates only the task status (for pause/resume).
func (s *Service) UpdateStatus(taskID, status string) error {
	_, err := s.coll.UpdateOne(context.Background(),
		bson.M{"_id": taskID},
		bson.M{"$set": bson.M{"status": status, "updated_at": time.Now()}},
	)
	return err
}
