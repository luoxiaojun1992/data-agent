package adksession

import "context"

// HistoryStore adapts a session *Service to the chat domain's narrow
// SessionHistoryStore contract (hard delete + clear history). It deliberately
// exposes only the two lifecycle operations so the service layer never depends
// on the full google.golang.org/adk/session.Service surface (SPEC-090 §5.2.3).
//
// The adapter is required because *Service already satisfies the ADK
// session.Service interface, whose Delete has signature
// Delete(ctx, *session.DeleteRequest) — a name collision with the narrow
// Delete(ctx, sessionID string).
type HistoryStore struct {
	svc *Service
}

// NewHistoryStore wraps svc into a SessionHistoryStore.
func NewHistoryStore(svc *Service) *HistoryStore {
	return &HistoryStore{svc: svc}
}

// Delete hard-deletes the session's ADK record, raw event stream and sub
// sessions (cascade).
func (h *HistoryStore) Delete(ctx context.Context, sessionID string) error {
	return h.svc.DeleteByID(ctx, sessionID)
}

// ClearHistory wipes the session's chat history while keeping its document.
func (h *HistoryStore) ClearHistory(ctx context.Context, sessionID string) error {
	return h.svc.ClearHistory(ctx, sessionID)
}
