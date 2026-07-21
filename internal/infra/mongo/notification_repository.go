package mongo

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// NotificationRepository implements repository.NotificationRepository backed by MongoDB.
type NotificationRepository struct {
	coll *mongo.Collection
}

// NewNotificationRepository creates a new NotificationRepository.
func NewNotificationRepository(db *mongo.Database) *NotificationRepository {
	return &NotificationRepository{coll: db.Collection(model.CollNotifications)}
}

// Create creates a new notification.
func (r *NotificationRepository) Create(ctx context.Context, n *model.Notification) error {
	n.ID = NewDomainID()
	_, err := r.coll.InsertOne(ctx, n)
	return err
}

// ListForUser returns notifications for a user with pagination.
func (r *NotificationRepository) ListForUser(ctx context.Context, userID string, skip, limit int64) ([]*model.Notification, int64, error) {
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
	var ns []*model.Notification
	if err := cursor.All(ctx, &ns); err != nil {
		return nil, 0, err
	}
	return ns, total, nil
}

// CountUnread returns the number of unread notifications for a user.
func (r *NotificationRepository) CountUnread(ctx context.Context, userID string) (int64, error) {
	return r.coll.CountDocuments(ctx, bson.M{"user_id": userID, "read": false})
}

// MarkRead marks a notification as read.
func (r *NotificationRepository) MarkRead(ctx context.Context, id string, userID string) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id, "user_id": userID}, bson.M{"$set": bson.M{"read": true}})
	return err
}

// MarkAllRead marks all notifications for a user as read.
func (r *NotificationRepository) MarkAllRead(ctx context.Context, userID string) error {
	_, err := r.coll.UpdateMany(ctx, bson.M{"user_id": userID}, bson.M{"$set": bson.M{"read": true}})
	return err
}

// Compile-time check.
var _ repository.NotificationRepository = (*NotificationRepository)(nil)
