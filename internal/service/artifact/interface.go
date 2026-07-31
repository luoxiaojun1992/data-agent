package artifact

import (
	"io"

	"github.com/luoxiaojun1992/data-agent/internal/domain/artifact"
)

//go:generate mockery --name Service --output ./mocks --outpkg mocks

// Service defines the artifact service contract. It directly orchestrates
// artifact metadata (MongoDB via ArtifactRepository) and file storage
// (SeaweedFS via FileRepository) — no extra abstraction in between.
type Service interface {
	Upload(userID, sessionID, taskID, name, mimeType string, reader io.Reader, persistent bool) (*artifact.Artifact, error)
	Download(id string) ([]byte, error)
	Delete(id string) error
	FindByID(id string) (*artifact.Artifact, error)
	ListBySession(sessionID string) ([]*artifact.Artifact, error)
	ListByUser(userID string, page, pageSize int) ([]*artifact.Artifact, int64, error)
	ListByTask(taskID string) ([]*artifact.Artifact, error)
}
