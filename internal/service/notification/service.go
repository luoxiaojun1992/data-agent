package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Service handles notification operations.
type Service struct {
	coll *mongo.Collection
}

// NewService creates a notification service.
func NewService(db *mongo.Database) *Service {
	return &Service{coll: db.Collection(model.CollNotifications)}
}

// Send sends a notification to specific users.
func (s *Service) Send(title, content, nType string, targetIDs []string) (*model.Notification, error) {
	n := &model.Notification{
		ID:        primitive.NewObjectID(),
		Title:     title,
		Content:   content,
		Type:      nType,
		TargetIDs: targetIDs,
		CreatedAt: time.Now(),
	}
	_, err := s.coll.InsertOne(context.Background(), n)
	if err != nil {
		return nil, fmt.Errorf("insert notification: %w", err)
	}
	return n, nil
}

// Broadcast sends a notification to all users.
func (s *Service) Broadcast(title, content, nType string) (*model.Notification, error) {
	n := &model.Notification{
		ID:        primitive.NewObjectID(),
		Title:     title,
		Content:   content,
		Type:      nType,
		TargetAll: true,
		CreatedAt: time.Now(),
	}
	_, err := s.coll.InsertOne(context.Background(), n)
	if err != nil {
		return nil, fmt.Errorf("broadcast: %w", err)
	}
	return n, nil
}

// ListForUser returns notifications visible to the user (targeted + broadcast), newest first.
func (s *Service) ListForUser(userID string, limit int64) ([]model.Notification, error) {
	if limit <= 0 {
		limit = 20
	}
	filter := bson.M{
		"$or": []bson.M{
			{"target_all": true},
			{"target_ids": userID},
		},
	}
	opts := options.Find().SetSort(bson.M{"created_at": -1}).SetLimit(limit)
	cursor, err := s.coll.Find(context.Background(), filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	var notifications []model.Notification
	if err := cursor.All(context.Background(), &notifications); err != nil {
		return nil, err
	}
	if notifications == nil {
		notifications = []model.Notification{}
	}
	return notifications, nil
}

// UnreadCount returns the number of unread notifications for a user.
func (s *Service) UnreadCount(userID string) (int64, error) {
	filter := bson.M{
		"read_by": bson.M{"$ne": userID},
		"$or": []bson.M{
			{"target_all": true},
			{"target_ids": userID},
		},
	}
	return s.coll.CountDocuments(context.Background(), filter)
}

// MarkRead marks a notification as read by the user.
func (s *Service) MarkRead(id string, userID string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id: %w", err)
	}
	_, err = s.coll.UpdateOne(context.Background(),
		bson.M{"_id": objID},
		bson.M{"$addToSet": bson.M{"read_by": userID}},
	)
	return err
}

// MarkAllRead marks all user-visible notifications as read.
func (s *Service) MarkAllRead(userID string) error {
	filter := bson.M{
		"$or": []bson.M{
			{"target_all": true},
			{"target_ids": userID},
		},
	}
	_, err := s.coll.UpdateMany(context.Background(), filter,
		bson.M{"$addToSet": bson.M{"read_by": userID}},
	)
	return err
}
