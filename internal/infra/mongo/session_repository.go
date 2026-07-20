package mongo

import (
	"context"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/repository"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SessionRepository implements repository.SessionRepository backed by MongoDB.
type SessionRepository struct {
	coll *mongo.Collection
}

// NewSessionRepository creates a new SessionRepository.
func NewSessionRepository(db *mongo.Database) *SessionRepository {
	return &SessionRepository{coll: db.Collection("sessions")}
}

func (r *SessionRepository) Create(ctx context.Context, s repository.SessionRecord) error {
	_, err := r.coll.InsertOne(ctx, s)
	return err
}

func (r *SessionRepository) Get(ctx context.Context, id string) (*repository.SessionRecord, error) {
	var s repository.SessionRecord
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&s)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SessionRepository) Renew(ctx context.Context, id string, newExpiry time.Time) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"expires_at": newExpiry}})
	return err
}

func (r *SessionRepository) ListByUser(ctx context.Context, userID string) ([]*repository.SessionRecord, error) {
	cursor, err := r.coll.Find(ctx, bson.M{"user_id": userID, "deleted_at": bson.M{"$exists": false}})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var sessions []*repository.SessionRecord
	if err := cursor.All(ctx, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

func (r *SessionRepository) Cleanup(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.coll.DeleteMany(ctx, bson.M{"expires_at": bson.M{"$lt": before}})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

func (r *SessionRepository) Delete(ctx context.Context, id string) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"deleted_at": time.Now()}})
	return err
}

func (r *SessionRepository) Restore(ctx context.Context, id string) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$unset": bson.M{"deleted_at": ""}})
	return err
}

func (r *SessionRepository) ListDeleted(ctx context.Context, before time.Time, limit int64) ([]*repository.SessionRecord, error) {
	opts := options.Find().SetLimit(limit)
	cursor, err := r.coll.Find(ctx, bson.M{"deleted_at": bson.M{"$lt": before}}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var sessions []*repository.SessionRecord
	if err := cursor.All(ctx, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

func (r *SessionRepository) SetRecoveryHours(ctx context.Context, hours int) error {
	_, err := r.coll.UpdateMany(ctx, bson.M{}, bson.M{"$set": bson.M{"recovery_hours": hours}})
	return err
}

var _ repository.SessionRepository = (*SessionRepository)(nil)
