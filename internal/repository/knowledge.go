package repository

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
)

//go:generate mockery --name KBRepository --output ./mocks --outpkg mocks

// KBRepository defines the data access contract for knowledge base documents and chunks.
type KBRepository interface {
	CreateDoc(ctx context.Context, doc *knowledge.KnowledgeDoc) error
	GetDoc(ctx context.Context, id string) (*knowledge.KnowledgeDoc, error)
	DeleteDoc(ctx context.Context, id string) error
	ListDocs(ctx context.Context, userID string, skip, limit int64) ([]*knowledge.KnowledgeDoc, int64, error)
	ListAllDocs(ctx context.Context) ([]*knowledge.KnowledgeDoc, error)
	UpdateDocStatus(ctx context.Context, id string, status knowledge.DocStatus, chunkCount int) error
	AddChunks(ctx context.Context, chunks []*knowledge.Chunk) error
	DeleteChunks(ctx context.Context, docID string) (int64, error)
	SearchChunks(ctx context.Context, query string, topK int) ([]*knowledge.SearchResult, error)
}

//go:generate mockery --name VectorRepository --output ./mocks --outpkg mocks

// VectorRepository defines the data access contract for vector search/upsert.
type VectorRepository interface {
	Upsert(ctx context.Context, collection string, vectors []VectorPoint) error
	Search(ctx context.Context, collection string, vector []float32, topK int, filter map[string]interface{}) ([]VectorSearchHit, error)
	DeleteCollection(ctx context.Context, collection string) error
}

// VectorPoint represents a single vector point for upsert.
type VectorPoint struct {
	ID       string
	Vector   []float32
	Metadata map[string]interface{}
}

// VectorSearchHit is one result from a vector search.
type VectorSearchHit struct {
	ID       string
	Score    float32
	Metadata map[string]interface{}
}

//go:generate mockery --name FileRepository --output ./mocks --outpkg mocks

// FileRepository defines the data access contract for file/blob storage.
type FileRepository interface {
	Upload(ctx context.Context, path string, data []byte, contentType string) error
	Download(ctx context.Context, path string) ([]byte, error)
	Delete(ctx context.Context, path string) error
}
