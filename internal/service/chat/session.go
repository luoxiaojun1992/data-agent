package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// Session represents a user session for Chat or Agent operations.
type Session struct {
	ID            string     `json:"id" bson:"_id"`
	UserID        string     `json:"user_id" bson:"user_id"`
	Type          string     `json:"type" bson:"type"`
	Status        string     `json:"status" bson:"status"`
	CreatedAt     time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" bson:"updated_at"`
	ExpiresAt     time.Time  `json:"expires_at" bson:"expires_at"`
	DeletedAt     *time.Time `json:"deleted_at,omitempty" bson:"deleted_at,omitempty"`
	RecoveryUntil *time.Time `json:"recovery_until,omitempty" bson:"recovery_until,omitempty"`
}

// Manager handles session lifecycle.
type Manager struct {
	repo repository.SessionRepository
	ttl  time.Duration
}

// NewManager creates a session manager.
func NewManager(repo repository.SessionRepository, ttl time.Duration) *Manager {
	return &Manager{repo: repo, ttl: ttl}
}

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
	rec := sessionToRecord(s)
	if err := m.repo.Create(context.Background(), rec); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return s, nil
}

func (m *Manager) Get(id string) (*Session, error) {
	rec, err := m.repo.Get(context.Background(), id)
	if err != nil {
		return nil, err
	}
	return recordToSession(rec), nil
}

func (m *Manager) Renew(id string) error {
	return m.repo.Renew(context.Background(), id, time.Now().Add(m.ttl))
}

func (m *Manager) Cleanup() (int64, error) {
	return m.repo.Cleanup(context.Background(), time.Now())
}

func (m *Manager) ListByUser(userID string) ([]*Session, error) {
	recs, err := m.repo.ListByUser(context.Background(), userID)
	if err != nil {
		return nil, err
	}
	sessions := make([]*Session, len(recs))
	for i, r := range recs {
		sessions[i] = recordToSession(r)
	}
	return sessions, nil
}

func (m *Manager) Delete(id string) error {
	return m.repo.Delete(context.Background(), id)
}

func (m *Manager) Restore(id string) error {
	return m.repo.Restore(context.Background(), id)
}

func (m *Manager) ListDeleted(before time.Time, limit int64) ([]*Session, error) {
	recs, err := m.repo.ListDeleted(context.Background(), before, limit)
	if err != nil {
		return nil, err
	}
	sessions := make([]*Session, len(recs))
	for i, r := range recs {
		sessions[i] = recordToSession(r)
	}
	return sessions, nil
}

func (m *Manager) SetRecoveryHours(hours int) error {
	return m.repo.SetRecoveryHours(context.Background(), hours)
}

func sessionToRecord(s *Session) repository.SessionRecord {
	r := repository.SessionRecord{
		ID:          s.ID,
		UserID:      s.UserID,
		Title:       s.Type,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
	return r
}

func recordToSession(r *repository.SessionRecord) *Session {
	return &Session{
		ID:      r.ID,
		UserID:  r.UserID,
		Type:    r.Title,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}
