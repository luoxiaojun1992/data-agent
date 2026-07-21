package knowledge

import (
	"time"
)

// DocStatus represents the processing status of a knowledge document.
type DocStatus string

const (
	StatusUploaded DocStatus = "uploaded"
	StatusParsing  DocStatus = "parsing"
	StatusIndexing DocStatus = "indexing"
	StatusReady    DocStatus = "ready"
	StatusFailed   DocStatus = "failed"
)

// KnowledgeDoc represents a knowledge base document metadata (MongoDB).
type KnowledgeDoc struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	Title        string    `json:"title"`
	FileName     string    `json:"file_name"`
	FileType     string    `json:"file_type"` // pdf, docx, xlsx, md, txt
	SizeBytes    int64     `json:"size_bytes"`
	Status       DocStatus `json:"status"`
	ChunkCount   int       `json:"chunk_count"`
	GridFSFileID string    `json:"gridfs_file_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// DocContent stores the parsed text content (MongoDB GridFS).
type DocContent struct {
	ID           string `json:"id"`
	DocID        string `json:"doc_id"`
	GridFSFileID string `json:"gridfs_file_id"`
	FileName     string `json:"file_name"`
	SizeBytes    int64  `json:"size_bytes"`
}

// Chunk represents a semantic chunk of document content (MongoDB).
type Chunk struct {
	ID        string    `json:"id"`
	DocID     string    `json:"doc_id"`
	Content   string    `json:"content"`
	ChunkIdx  int       `json:"chunk_idx"`
	CharCount int       `json:"char_count"`
	MilvusID  int64     `json:"milvus_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// IndexTask represents an async indexing task (MongoDB).
type IndexTask struct {
	ID        string    `json:"id"`
	DocID     string    `json:"doc_id"`
	Status    string    `json:"status"`   // pending, running, completed, failed
	Progress  int       `json:"progress"` // 0-100
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SearchResult represents a single search result with score.
type SearchResult struct {
	ChunkID  string  `json:"chunk_id"`
	DocID    string  `json:"doc_id"`
	DocTitle string  `json:"doc_title"`
	Content  string  `json:"content"`
	Score    float64 `json:"score"`
	Source   string  `json:"source"` // "milvus" or "fulltext"
}

// AggregationLayer represents a multi-dimension aggregation definition.
type AggregationLayer struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Dimensions []string  `json:"dimensions"`
	Metrics    []string  `json:"metrics"`
	CreatedAt  time.Time `json:"created_at"`
}
