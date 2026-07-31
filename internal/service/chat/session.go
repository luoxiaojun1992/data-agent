package chat

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// workspaceBase is the root directory for session workspaces.
const workspaceBase = "data-agent-sessions"

// SessionWorkspace returns the isolated temp directory for a session.
// Exported for use by tools (pptx_generator, save_artifact) that need
// to resolve session-scoped file paths.
func SessionWorkspace(sessionID string) string {
	return filepath.Join(os.TempDir(), workspaceBase, sessionID)
}

// ensureWorkspace creates the workspace directory if it doesn't exist.
func ensureWorkspace(sessionID string) error {
	return os.MkdirAll(SessionWorkspace(sessionID), 0700)
}

// removeWorkspace deletes the entire session workspace (best-effort).
func removeWorkspace(sessionID string) {
	_ = os.RemoveAll(SessionWorkspace(sessionID))
}

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

func (m *Manager) Create(userID, sessionType, modelID string) (*domainchat.Session, error) {
	now := time.Now()
	s := &domainchat.Session{
		ID:        "sess_" + uuid.New().String(),
		UserID:    userID,
		Type:      sessionType,
		ModelID:   modelID,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: now.Add(m.ttl),
	}
	// Create isolated workspace for agent-generated files.
	if err := ensureWorkspace(s.ID); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	rec := sessionToRecord(s)
	if err := m.repo.Create(context.Background(), rec); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return s, nil
}

// CreateTaskSession creates a session flagged as an autonomous task run.
// It shares the chat-session infrastructure (ADK session service, registry
// runtime resolution) but the IsTask flag tells downstream consumers this
// is not a real-time user conversation.
func (m *Manager) CreateTaskSession(userID, modelID string) (*domainchat.Session, error) {
	now := time.Now()
	s := &domainchat.Session{
		ID:        "sess_" + uuid.New().String(),
		UserID:    userID,
		Type:      "task",
		ModelID:   modelID,
		IsTask:    true,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: now.Add(m.ttl),
	}
	rec := sessionToRecord(s)
	if err := m.repo.Create(context.Background(), rec); err != nil {
		return nil, fmt.Errorf("create task session: %w", err)
	}
	return s, nil
}

// CreateFeishuSession creates a session flagged for Feishu IM bot integration.
func (m *Manager) CreateFeishuSession(userID, modelID string) (*domainchat.Session, error) {
	now := time.Now()
	s := &domainchat.Session{
		ID:        "sess_" + uuid.New().String(),
		UserID:    userID,
		Type:      "chat",
		ModelID:   modelID,
		IsFeishu:  true,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: now.Add(m.ttl),
	}
	rec := sessionToRecord(s)
	if err := m.repo.Create(context.Background(), rec); err != nil {
		return nil, fmt.Errorf("create feishu session: %w", err)
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

// ListByUserPaged returns paginated sessions sorted by created_at DESC.
func (m *Manager) ListByUserPaged(userID string, page, pageSize int) ([]*domainchat.Session, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	skip := int64((page - 1) * pageSize)
	recs, total, err := m.repo.ListByUserPaged(context.Background(), userID, skip, int64(pageSize))
	if err != nil {
		return nil, 0, err
	}
	sessions := make([]*domainchat.Session, len(recs))
	for i, r := range recs {
		sessions[i] = recordToSession(r)
	}
	return sessions, total, nil
}

func (m *Manager) Delete(id string) error {
	// Clean workspace before deleting from DB.
	removeWorkspace(id)
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

func (m *Manager) SetTitle(id, title string) error {
	return m.repo.SetTitle(context.Background(), id, title)
}

func sessionToRecord(s *domainchat.Session) repository.SessionRecord {
	r := repository.SessionRecord{
		ID:        s.ID,
		UserID:    s.UserID,
		Title:     s.Title,
		ModelID:   s.ModelID,
		IsTask:    s.IsTask,
		IsFeishu:  s.IsFeishu,
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
		Type:      "chat",
		Title:     r.Title,
		ModelID:   r.ModelID,
		IsTask:    r.IsTask,
		IsFeishu:  r.IsFeishu,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
		ExpiresAt: r.ExpiresAt,
	}
}
