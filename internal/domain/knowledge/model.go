package knowledge

import (
	"time"
)

// DocStatus represents the processing status of a knowledge document.
type DocStatus string

const (
	StatusUploaded   DocStatus = "uploaded"
	StatusParsing    DocStatus = "parsing"
	StatusIndexing   DocStatus = "indexing"
	StatusReady      DocStatus = "ready"
	StatusFailed     DocStatus = "failed"
)

// KnowledgeDoc represents a knowledge base document metadata (MongoDB).
type KnowledgeDoc struct {
	ID          string    `bson:"_id" json:"id"`
	UserID      string    `bson:"user_id" json:"user_id"`
	Title       string    `bson:"title" json:"title"`
	FileName    string    `bson:"file_name" json:"file_name"`
	FileType    string    `bson:"file_type" json:"file_type"` // pdf, docx, xlsx, md, txt
	SizeBytes   int64     `bson:"size_bytes" json:"size_bytes"`
	Status      DocStatus `bson:"status" json:"status"`
	ChunkCount  int       `bson:"chunk_count" json:"chunk_count"`
	CreatedAt   time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time `bson:"updated_at" json:"updated_at"`
}

// DocContent stores the parsed text content (MongoDB GridFS).
type DocContent struct {
	ID           string `bson:"_id" json:"id"`
	DocID        string `bson:"doc_id" json:"doc_id"`
	GridFSFileID string `bson:"gridfs_file_id" json:"gridfs_file_id"`
	FileName     string `bson:"file_name" json:"file_name"`
	SizeBytes    int64  `bson:"size_bytes" json:"size_bytes"`
}

// Chunk represents a semantic chunk of document content (MongoDB).
type Chunk struct {
	ID        string    `bson:"_id" json:"id"`
	DocID     string    `bson:"doc_id" json:"doc_id"`
	Content   string    `bson:"content" json:"content"`
	ChunkIdx  int       `bson:"chunk_idx" json:"chunk_idx"`
	CharCount int       `bson:"char_count" json:"char_count"`
	MilvusID  int64     `bson:"milvus_id,omitempty" json:"milvus_id,omitempty"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
}

// IndexTask represents an async indexing task (MongoDB).
type IndexTask struct {
	ID        string    `bson:"_id" json:"id"`
	DocID     string    `bson:"doc_id" json:"doc_id"`
	Status    string    `bson:"status" json:"status"` // pending, running, completed, failed
	Progress  int       `bson:"progress" json:"progress"` // 0-100
	Error     string    `bson:"error,omitempty" json:"error,omitempty"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
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
	ID         string    `bson:"_id" json:"id"`
	Name       string    `bson:"name" json:"name"`
	Dimensions []string  `bson:"dimensions" json:"dimensions"`
	Metrics    []string  `bson:"metrics" json:"metrics"`
	CreatedAt  time.Time `bson:"created_at" json:"created_at"`
}
