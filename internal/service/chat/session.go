package chat

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Session represents a user session for Chat or Agent operations.
type Session struct {
	ID        string    `json:"id" bson:"_id"`
	UserID    string    `json:"user_id" bson:"user_id"`
	Type      string    `json:"type" bson:"type"` // "chat" or "agent"
	Status    string    `json:"status" bson:"status"` // "active", "expired"
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
	ExpiresAt time.Time `json:"expires_at" bson:"expires_at"`
}

// Manager handles session lifecycle.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	ttl      time.Duration
}

// NewManager creates a session manager with the given TTL.
func NewManager(ttl time.Duration) *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		ttl:      ttl,
	}
}

// Create creates a new session.
func (m *Manager) Create(userID, sessionType string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

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
	m.sessions[s.ID] = s
	return s, nil
}

// Get retrieves a session by ID.
func (m *Manager) Get(id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, exists := m.sessions[id]
	if !exists {
		return nil, fmt.Errorf("session %q not found", id)
	}
	if time.Now().After(s.ExpiresAt) {
		s.Status = "expired"
		return nil, fmt.Errorf("session %q expired", id)
	}
	return s, nil
}

// Renew extends the session TTL.
func (m *Manager) Renew(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, exists := m.sessions[id]
	if !exists {
		return fmt.Errorf("session %q not found", id)
	}
	s.UpdatedAt = time.Now()
	s.ExpiresAt = time.Now().Add(m.ttl)
	return nil
}

// Cleanup removes expired sessions.
func (m *Manager) Cleanup(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, s := range m.sessions {
		if now.After(s.ExpiresAt) {
			delete(m.sessions, id)
		}
	}
}

// ListByUser returns active sessions for a user.
func (m *Manager) ListByUser(userID string) []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*Session
	now := time.Now()
	for _, s := range m.sessions {
		if s.UserID == userID && !now.After(s.ExpiresAt) {
			result = append(result, s)
		}
	}
	return result
}

// Delete removes a session by ID.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.sessions[id]; !exists {
		return fmt.Errorf("session %q not found", id)
	}
	delete(m.sessions, id)
	return nil
}
