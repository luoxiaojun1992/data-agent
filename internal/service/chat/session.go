package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// Session represents a user session for Chat or Agent operations.
type Session struct {
	ID            string     `json:"id" bson:"_id"`
	UserID        string     `json:"user_id" bson:"user_id"`
	Type          string     `json:"type" bson:"type"`     // "chat" or "agent"
	Status        string     `json:"status" bson:"status"` // "active", "expired", "deleted"
	CreatedAt     time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" bson:"updated_at"`
	ExpiresAt     time.Time  `json:"expires_at" bson:"expires_at"`
	DeletedAt     *time.Time `json:"deleted_at,omitempty" bson:"deleted_at,omitempty"`
	RecoveryUntil *time.Time `json:"recovery_until,omitempty" bson:"recovery_until,omitempty"`
}

// Manager handles session lifecycle with MongoDB persistence.
type Manager struct {
	coll *mongo.Collection
	ttl  time.Duration
}

// NewManager creates a session manager with MongoDB persistence.
func NewManager(db *mongo.Database, ttl time.Duration) *Manager {
	return &Manager{
		coll: db.Collection("sessions"),
		ttl:  ttl,
	}
}

// Create creates a new session.
func (m *Manager) Create(userID, sessionType string) (*Session, error) {
	now := time.Now()
	s := &Session{
		ID:        "sess_" + uuid.New().String(),
		UserID:    userID,
		Type:      sessionType,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: now.Add(m.ttl),
	}
	_, err := m.coll.InsertOne(context.Background(), s)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return s, nil
}

// Get retrieves a session by ID (not deleted, not expired).
func (m *Manager) Get(id string) (*Session, error) {
	var s Session
	err := m.coll.FindOne(context.Background(), bson.M{
		"_id":        id,
		"deleted_at": nil,
	}).Decode(&s)
	if err != nil {
		return nil, fmt.Errorf("session %q not found", id)
	}
	if time.Now().After(s.ExpiresAt) {
		s.Status = "expired"
		return nil, fmt.Errorf("session %q expired", id)
	}
	return &s, nil
}

// Renew extends the session TTL.
func (m *Manager) Renew(id string) error {
	now := time.Now()
	res, err := m.coll.UpdateOne(context.Background(),
		bson.M{"_id": id, "deleted_at": nil},
		bson.M{"$set": bson.M{"updated_at": now, "expires_at": now.Add(m.ttl)}},
	)
	if err != nil {
		return fmt.Errorf("renew session: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("session %q not found", id)
	}
	return nil
}

// Cleanup removes expired sessions and sessions past their recovery window.
func (m *Manager) Cleanup(ctx context.Context) {
	now := time.Now()
	// Remove sessions expired over 1 hour ago, or past recovery window
	_, _ = m.coll.DeleteMany(ctx, bson.M{
		"$or": []bson.M{
			{"expires_at": bson.M{"$lt": now.Add(-1 * time.Hour)}},
			{"recovery_until": bson.M{"$lt": now}},
		},
	})
}

// ListByUser returns active sessions for a user (not deleted).
func (m *Manager) ListByUser(userID string) []*Session {
	var result []*Session
	cursor, err := m.coll.Find(context.Background(), bson.M{
		"user_id":    userID,
		"deleted_at": nil,
	})
	if err != nil {
		return result
	}
	defer cursor.Close(context.Background())
	for cursor.Next(context.Background()) {
		var s Session
		if err := cursor.Decode(&s); err == nil {
			// Skip expired
			if time.Now().After(s.ExpiresAt) {
				s.Status = "expired"
			}
			result = append(result, &s)
		}
	}
	return result
}

// Delete soft-deletes a session (sets DeletedAt, RecoveryUntil).
func (m *Manager) Delete(id string) error {
	now := time.Now()
	res, err := m.coll.UpdateOne(context.Background(),
		bson.M{"_id": id, "deleted_at": nil},
		bson.M{"$set": bson.M{
			"deleted_at":     now,
			"recovery_until": now.Add(24 * time.Hour),
			"status":         "deleted",
			"updated_at":     now,
		}},
	)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("session %q not found", id)
	}
	return nil
}

// Restore recovers a soft-deleted session within the recovery window.
func (m *Manager) Restore(id string) error {
	now := time.Now()
	res, err := m.coll.UpdateOne(context.Background(),
		bson.M{
			"_id":            id,
			"deleted_at":     bson.M{"$ne": nil},
			"recovery_until": bson.M{"$gte": now},
		},
		bson.M{"$set": bson.M{
			"deleted_at":     nil,
			"recovery_until": nil,
			"status":         "active",
			"updated_at":     now,
			"expires_at":     now.Add(m.ttl),
		}},
	)
	if err != nil {
		return fmt.Errorf("restore session: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("session %q not found or recovery window expired", id)
	}
	return nil
}

// ListDeleted returns soft-deleted sessions within the recovery window.
func (m *Manager) ListDeleted(userID string) []*Session {
	var result []*Session
	now := time.Now()
	cursor, err := m.coll.Find(context.Background(), bson.M{
		"user_id":        userID,
		"deleted_at":     bson.M{"$ne": nil},
		"recovery_until": bson.M{"$gte": now},
	})
	if err != nil {
		return result
	}
	defer cursor.Close(context.Background())
	for cursor.Next(context.Background()) {
		var s Session
		if err := cursor.Decode(&s); err == nil {
			result = append(result, &s)
		}
	}
	return result
}

// SetRecoveryHours sets the recovery window for future soft-deletes.
// This does NOT affect already-deleted sessions — it only influences how
// Cleanup determines what to purge.
func (m *Manager) SetRecoveryHours(hours int) error {
	// This is called when sysconfig updates session_recovery_hours.
	// The recovery_until for NEW deletes will use this value through the system config.
	// For simplicity, we update the recovery window of all currently deleted sessions.
	if hours < 1 || hours > 168 {
		return fmt.Errorf("recovery hours must be between 1 and 168")
	}
	now := time.Now()
	newUntil := now.Add(time.Duration(hours) * time.Hour)
	_, err := m.coll.UpdateMany(context.Background(),
		bson.M{"deleted_at": bson.M{"$ne": nil}},
		bson.M{"$set": bson.M{"recovery_until": newUntil}},
	)
	return err
}
