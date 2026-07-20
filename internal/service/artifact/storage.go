package artifact

import (
	"context"
	"fmt"
	"io"

	"github.com/luoxiaojun1992/data-agent/internal/domain/artifact"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// Storage combines file storage with MongoDB metadata.
type Storage struct {
	files  repository.FileRepository
	meta   repository.ArtifactRepository
}

// NewStorage creates a new artifact storage adapter.
func NewStorage(files repository.FileRepository, meta repository.ArtifactRepository) *Storage {
	return &Storage{files: files, meta: meta}
}

// Upload stores a file and creates metadata.
func (s *Storage) Upload(userID, sessionID, taskID, name, mimeType string, reader io.Reader, persistent bool) (*artifact.Artifact, error) {
	storagePath := fmt.Sprintf("artifacts/%s/%s/%s", userID, sessionID, name)

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read upload: %w", err)
	}
	if err := s.files.Upload(context.Background(), storagePath, data, mimeType); err != nil {
		return nil, fmt.Errorf("file upload: %w", err)
	}

	art := artifact.NewArtifact(userID, sessionID, taskID, name, mimeType, storagePath, int64(len(data)), persistent)
	if err := s.meta.Create(context.Background(), art); err != nil {
		_ = s.files.Delete(context.Background(), storagePath)
		return nil, fmt.Errorf("insert artifact metadata: %w", err)
	}

	return art, nil
}

// Download retrieves a file from storage.
func (s *Storage) Download(storagePath string) ([]byte, error) {
	return s.files.Download(context.Background(), storagePath)
}

// Delete removes an artifact from storage and metadata.
func (s *Storage) Delete(id, storagePath string) error {
	if err := s.meta.Delete(context.Background(), id); err != nil {
		return fmt.Errorf("delete metadata: %w", err)
	}
	_ = s.files.Delete(context.Background(), storagePath)
	return nil
}

// FindByID returns artifact metadata by ID.
func (s *Storage) FindByID(id string) (*artifact.Artifact, error) {
	return s.meta.FindByID(context.Background(), id)
}

// ListBySession returns artifacts for a session.
func (s *Storage) ListBySession(sessionID string) ([]*artifact.Artifact, error) {
	return s.meta.ListBySession(context.Background(), sessionID)
}

// ListByTask returns artifacts for a task.
func (s *Storage) ListByTask(taskID string) ([]*artifact.Artifact, error) {
	return s.meta.ListByTask(context.Background(), taskID)
}
