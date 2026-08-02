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
	GetDoc(id string) (*knowledge.KnowledgeDoc, error)
	DeleteDoc(id string) error
	ListDocs(userID string, page, pageSize int) ([]*knowledge.KnowledgeDoc, int64, error)
	ListDocsByVisibility(userID string, isSystemAdmin bool, page, pageSize int) ([]*knowledge.KnowledgeDoc, int64, error)
	ListAllDocs(page, pageSize int) ([]*knowledge.KnowledgeDoc, int64, error)
	AddChunks(docID string, texts []string) error
	Search(userID, query string, topK int, isSystemAdmin bool) ([]knowledge.SearchResult, error)
	SetPublicFlag(ctx context.Context, docID string, isPublic bool) error
	UploadFile(fileName, contentType string, reader io.Reader) (string, error)
}

var _ KnowledgeService = (*Service)(nil)
