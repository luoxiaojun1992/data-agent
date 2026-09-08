package chat

import (
	"context"
	"fmt"
	"log"
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
	repo         repository.SessionRepository
	ttl          time.Duration
	historyStore domainchat.SessionHistoryStore
}

// NewManager creates a session manager. historyStore backs HardDelete and
// ClearHistory (the ADK-side chat history); it may be nil in tests where those
// operations are not exercised.
func NewManager(repo repository.SessionRepository, ttl time.Duration, historyStore domainchat.SessionHistoryStore) *Manager {
	return &Manager{repo: repo, ttl: ttl, historyStore: historyStore}
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
	ctx := context.Background()
	// List active (non-archived) expired sessions. Archived sessions are exempt
	// from cleanup and remain until manually restored or hard-deleted.
	recs, err := m.repo.ListExpired(ctx, time.Now())
	if err != nil {
		return 0, err
	}
	var deleted int64
	for _, rec := range recs {
		// Reuse the full cascade path (sessions + workspace + ADK history + sub
		// sessions); artifact/memory are never touched.
		if err := m.HardDelete(rec.ID); err != nil {
			log.Printf("[session] cleanup delete %s: %v", rec.ID, err)
			continue
		}
		deleted++
	}
	return deleted, nil
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
// q filters by title/id at the DB layer (SPEC-075); empty = no filter.
func (m *Manager) ListByUserPaged(userID string, q string, page, pageSize int) ([]*domainchat.Session, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	skip := int64((page - 1) * pageSize)
	recs, total, err := m.repo.ListByUserPaged(context.Background(), userID, q, skip, int64(pageSize))
	if err != nil {
		return nil, 0, err
	}
	sessions := make([]*domainchat.Session, len(recs))
	for i, r := range recs {
		sessions[i] = recordToSession(r)
	}
	return sessions, total, nil
}

// Delete archives (soft-deletes) a session: sets deleted_at, keeps the
// workspace and chat history for a complete restore, and never auto-deletes
// via TTL (SPEC-090).
func (m *Manager) Delete(id string) error {
	return m.repo.Delete(context.Background(), id)
}

// HardDelete permanently removes a session: the sessions record, its workspace
// directory, and its ADK-side chat history (compacted events + raw event stream
// + any sub-agent sessions). artifact/memory are permanent products and are
// never cascaded (SPEC-090).
func (m *Manager) HardDelete(id string) error {
	if err := m.repo.HardDelete(context.Background(), id); err != nil {
		return err
	}
	removeWorkspace(id)
	if m.historyStore != nil {
		if err := m.historyStore.Delete(context.Background(), id); err != nil {
			return err
		}
	}
	return nil
}

// ClearHistory wipes a session's chat history (compacted + raw) while keeping
// the session record and its workspace (SPEC-090).
func (m *Manager) ClearHistory(id string) error {
	if m.historyStore == nil {
		return nil
	}
	return m.historyStore.ClearHistory(context.Background(), id)
}

func (m *Manager) Restore(id string) error {
	return m.repo.Restore(context.Background(), id)
}

func (m *Manager) ListDeleted(userID string) ([]*domainchat.Session, error) {
	recs, err := m.repo.ListDeleted(context.Background(), userID, 100)
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
		Status:    "active",
		IsTask:    r.IsTask,
		IsFeishu:  r.IsFeishu,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
		ExpiresAt: r.ExpiresAt,
		DeletedAt: r.DeletedAt,
	}
}
