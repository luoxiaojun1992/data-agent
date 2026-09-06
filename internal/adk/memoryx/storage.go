package memoryx

import (
	"context"

	"github.com/ieshan/adk-go-memory/adapter"
	"github.com/ieshan/idx"
)

// Storage is the abstract interface for memory observation storage.
type Storage interface {
	Store(ctx context.Context, obs *adapter.Observation) error
	GetByID(ctx context.Context, id idx.ID) (*adapter.Observation, error)
	Search(ctx context.Context, opts *adapter.SearchOptions) ([]adapter.SearchResult, error)
	List(ctx context.Context, userID, query string, page, pageSize int) ([]adapter.Observation, int64, error)
	// ListRecent returns observations for a user sorted by created_at DESC,
	// paginated with skip/limit. Returns the total count for has_more. Unlike
	// List (which sorts by updated_at — refreshed on merge/upsert), this is the
	// canonical "read today's memories" pager (SPEC-086 §5.1).
	ListRecent(ctx context.Context, userID string, limit, offset int) ([]adapter.Observation, int64, error)
}
