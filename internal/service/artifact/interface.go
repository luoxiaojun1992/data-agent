package artifact

import (
	"io"

	"github.com/luoxiaojun1992/data-agent/internal/domain/artifact"
)

//go:generate mockery --name StorageService --output ./mocks --outpkg mocks

// StorageService defines the artifact storage service contract.
type StorageService interface {
	Upload(userID, sessionID, taskID, name, mimeType string, reader io.Reader, persistent bool) (*artifact.Artifact, error)
	Download(id string) ([]byte, error)
	Delete(id string) error
	FindByID(id string) (*artifact.Artifact, error)
	ListBySession(sessionID string) ([]*artifact.Artifact, error)
	ListByTask(taskID string) ([]*artifact.Artifact, error)
}

var _ StorageService = (*Storage)(nil)
