package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// Manager handles session lifecycle. It implements domain/chat.SessionService.
type Manager struct {
	repo repository.SessionRepository
	ttl  time.Duration
}

// NewManager creates a session manager.
func NewManager(repo repository.SessionRepository, ttl time.Duration) *Manager {
	return &Manager{repo: repo, ttl: ttl}
}

// ensure Manager satisfies the domain SessionService contract.
var _ domainchat.SessionService = (*Manager)(nil)

func (m *Manager) Create(userID, sessionType string) (*domainchat.Session, error) {
	now := time.Now()
	s := &domainchat.Session{
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

func (m *Manager) Get(id string) (*domainchat.Session, error) {
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

func (m *Manager) ListByUser(userID string) ([]*domainchat.Session, error) {
	recs, err := m.repo.ListByUser(context.Background(), userID)
	if err != nil {
		return nil, err
	}
	sessions := make([]*domainchat.Session, len(recs))
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

func (m *Manager) ListDeleted(before time.Time, limit int64) ([]*domainchat.Session, error) {
	recs, err := m.repo.ListDeleted(context.Background(), before, limit)
	if err != nil {
		return nil, err
	}
	sessions := make([]*domainchat.Session, len(recs))
	for i, r := range recs {
		sessions[i] = recordToSession(r)
	}
	return sessions, nil
}

func (m *Manager) SetRecoveryHours(hours int) error {
	return m.repo.SetRecoveryHours(context.Background(), hours)
}

func sessionToRecord(s *domainchat.Session) repository.SessionRecord {
	r := repository.SessionRecord{
		ID:        s.ID,
		UserID:    s.UserID,
		Title:     s.Type,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
		ExpiresAt: s.ExpiresAt,
	}
	return r
}

func recordToSession(r *repository.SessionRecord) *domainchat.Session {
	return &domainchat.Session{
		ID:        r.ID,
		UserID:    r.UserID,
		Type:      r.Title,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
		ExpiresAt: r.ExpiresAt,
	}
}
