package mongo

import (
	"context"
	"regexp"
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
	opts := options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}})
	filter := chatSourceFilter(userID)
	filter["deleted_at"] = bson.M{"$exists": false}
	cursor, err := r.coll.Find(ctx, filter, opts)
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

// ListByUserPaged returns paginated sessions sorted by updated_at DESC.
// q filters by title/_id (case-insensitive $regex, quote-meta escaped).
func (r *SessionRepository) ListByUserPaged(ctx context.Context, userID string, q string, skip, limit int64) ([]*repository.SessionRecord, int64, error) {
	filter := chatSourceFilter(userID)
	filter["deleted_at"] = bson.M{"$exists": false}
	if q != "" {
		qre := bson.M{"$regex": regexp.QuoteMeta(q), "$options": "i"}
		filter["$or"] = []bson.M{
			{"title": qre},
			{"_id": qre},
		}
	}
	total, _ := r.coll.CountDocuments(ctx, filter)
	opts := options.Find().
		SetSort(bson.D{{Key: "updated_at", Value: -1}}).
		SetSkip(skip).
		SetLimit(limit)
	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var sessions []*repository.SessionRecord
	if err := cursor.All(ctx, &sessions); err != nil {
		return nil, 0, err
	}
	return sessions, total, nil
}

func (r *SessionRepository) Delete(ctx context.Context, id string) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"deleted_at": time.Now()}})
	return err
}

func (r *SessionRepository) HardDelete(ctx context.Context, id string) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *SessionRepository) Restore(ctx context.Context, id string) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$unset": bson.M{"deleted_at": ""}})
	return err
}

// ListDeleted returns the user's archived sessions (deleted_at exists), sorted
// most-recently-archived first (SPEC-090).
func (r *SessionRepository) ListDeleted(ctx context.Context, userID string, limit int64) ([]*repository.SessionRecord, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "deleted_at", Value: -1}}).
		SetLimit(limit)
	filter := chatSourceFilter(userID)
	filter["deleted_at"] = bson.M{"$exists": true}
	cursor, err := r.coll.Find(ctx, filter, opts)
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

// chatSourceFilter returns the base filter shared by every chat-session list
// query: scoped to the user and excluding task/feishu sessions (SPEC-104 D3).
// $ne:true also matches legacy documents where the omitempty field was never
// written, so pre-existing sessions are correctly included in the chat list.
func chatSourceFilter(userID string) bson.M {
	return bson.M{
		"user_id":   userID,
		"is_task":   bson.M{"$ne": true},
		"is_feishu": bson.M{"$ne": true},
	}
}

// ListExpired returns active (non-archived) sessions whose expires_at is before
// the given time. Archived sessions (deleted_at exists) are exempt from cleanup
// and never returned (SPEC-090 §5.1).
func (r *SessionRepository) ListExpired(ctx context.Context, before time.Time) ([]*repository.SessionRecord, error) {
	cursor, err := r.coll.Find(ctx, bson.M{
		"expires_at": bson.M{"$lt": before},
		"deleted_at": bson.M{"$exists": false},
	})
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

func (r *SessionRepository) SetTitle(ctx context.Context, id, title string) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{
		"title":      title,
		"updated_at": time.Now(),
	}})
	return err
}

var _ repository.SessionRepository = (*SessionRepository)(nil)
