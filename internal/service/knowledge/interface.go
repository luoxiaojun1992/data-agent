package knowledge

import (
	"context"
	"io"

	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
)

//go:generate mockery --name KnowledgeService --output ./mocks --outpkg mocks

// KnowledgeService defines the knowledge management service contract.
type KnowledgeService interface {
	CreateDoc(userID, title, fileName, fileType string, sizeBytes int64, gridFSFileID string) (*knowledge.KnowledgeDoc, error)
	// CreateTextDoc creates a plain-text KB doc end-to-end: length check →
	// PII redaction → GridFS upload → CreateDoc → async index enqueue. Used by
	// kb_create_doc (SPEC-086). Returns the created doc (status=uploaded) without
	// waiting for indexing.
	CreateTextDoc(ctx context.Context, userID, title, text string) (*knowledge.KnowledgeDoc, error)
	GetDoc(id, userID string, isSystemAdmin bool) (*knowledge.KnowledgeDoc, error)
	DeleteDoc(id, userID string, isSystemAdmin bool) error
	ListDocs(userID string, page, pageSize int) ([]*knowledge.KnowledgeDoc, int64, error)
	ListDocsByVisibility(userID string, isSystemAdmin bool, q string, page, pageSize int) ([]*knowledge.KnowledgeDoc, int64, error)
	ListAllDocs(page, pageSize int) ([]*knowledge.KnowledgeDoc, int64, error)
	AddChunks(docID string, texts []string) error
	Search(userID, query string, topK int, isSystemAdmin bool) ([]knowledge.SearchResult, error)
	SetPublicFlag(ctx context.Context, docID string, isPublic bool, userID string, isSystemAdmin bool) error
	UploadFile(fileName, contentType string, reader io.Reader) (string, error)
	RedactText(ctx context.Context, text string) (string, error)
}

var _ KnowledgeService = (*Service)(nil)
