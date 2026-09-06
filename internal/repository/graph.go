// Package repository holds data-access contracts shared across the codebase.
// GraphRepository is the graph-database contract used by both the KB indexing
// pipeline (write side) and the knowledge_graph_search skill (read side).
// (SPEC-070)
package repository

import "context"

// GraphChunk is a chunk node written to the graph store.
type GraphChunk struct {
	ChunkID   string
	DocID     string
	ChunkIdx  int
	CreatorID string // = userId，自定义字段，供权限过滤
	IsPublic  bool   // 自定义字段，供可见性过滤
	CharCount int
}

// RelatedRef is one RELATED_TO edge target with its similarity score.
type RelatedRef struct {
	ChunkID string
	Score   float64
}

// GraphFilter is the visibility filter applied to graph queries, mirroring
// the KB visibility rules.
type GraphFilter struct {
	CreatorID     string
	IsSystemAdmin bool
}

// GraphNode is one related chunk returned by a graph query. Content is filled
// by the caller (skill) via Qdrant lookup — the graph stores only metadata.
type GraphNode struct {
	ChunkID string
	DocID   string
	Score   float64
	Content string
}

// GraphRepository abstracts the graph database (ArcadeDB via Bolt). It is a
// shared component: KB indexing writes through it, the graph search skill
// reads through it. Implementations must be idempotent for all mutations.
type GraphRepository interface {
	// EnsureSchema idempotently creates constraints/indexes (startup seed).
	EnsureSchema(ctx context.Context) error
	// UpsertChunk writes or updates a chunk node (MERGE by chunk_id).
	UpsertChunk(ctx context.Context, c GraphChunk) error
	// LinkRelated creates RELATED_TO edges from chunkID to each ref (score on
	// the edge). Edges are created idempotently (MERGE).
	LinkRelated(ctx context.Context, chunkID string, refs []RelatedRef) error
	// DeleteChunk removes a chunk node and its edges (idempotent).
	DeleteChunk(ctx context.Context, chunkID string) error
	// DeleteByDocID removes all chunk nodes of a document and their edges
	// (idempotent, DETACH DELETE).
	DeleteByDocID(ctx context.Context, docID string) error
	// SetDocPublic updates the is_public flag on all chunk nodes of a document
	// (idempotent). Keeps graph visibility in sync with the KB doc's shared
	// state (SPEC-091).
	SetDocPublic(ctx context.Context, docID string, isPublic bool) error
	// QueryTopN returns up to topN related chunks of the anchor chunk, sorted
	// by score desc, filtered by visibility (system_admin sees all; regular
	// users see their own or public chunks).
	QueryTopN(ctx context.Context, anchorID string, topN int, filter GraphFilter) ([]GraphNode, error)
}
