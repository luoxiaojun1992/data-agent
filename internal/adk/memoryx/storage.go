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
}
