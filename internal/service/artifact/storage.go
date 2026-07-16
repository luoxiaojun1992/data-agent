package artifact

import (
	"context"
	"fmt"
	"io"

	"github.com/luoxiaojun1992/data-agent/internal/domain/artifact"
	"github.com/luoxiaojun1992/data-agent/internal/infra/seaweedfs"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const collArtifacts = "artifacts"

// Storage combines SeaweedFS storage with MongoDB metadata.
type Storage struct {
	sw   *seaweedfs.Client
	coll *mongo.Collection
}

// NewStorage creates a new artifact storage adapter.
func NewStorage(sw *seaweedfs.Client, db *mongo.Database) *Storage {
	return &Storage{
		sw:   sw,
		coll: db.Collection(collArtifacts),
	}
}

// Upload stores a file in SeaweedFS and creates MongoDB metadata.
func (s *Storage) Upload(userID, sessionID, taskID, name, mimeType string, reader io.Reader, persistent bool) (*artifact.Artifact, error) {
	storagePath := fmt.Sprintf("artifacts/%s/%s/%s", userID, sessionID, name)

	size, err := s.sw.Upload(storagePath, reader)
	if err != nil {
		return nil, fmt.Errorf("seaweedfs upload: %w", err)
	}

	art := artifact.NewArtifact(userID, sessionID, taskID, name, mimeType, storagePath, size, persistent)

	_, err = s.coll.InsertOne(context.Background(), art)
	if err != nil {
		// Best-effort cleanup: remove from SeaweedFS if MongoDB insert fails
		_ = s.sw.Delete(storagePath)
		return nil, fmt.Errorf("insert artifact metadata: %w", err)
	}

	return art, nil
}

// Download retrieves file content from SeaweedFS.
func (s *Storage) Download(artifactID string) ([]byte, *artifact.Artifact, error) {
	art, err := s.FindByID(artifactID)
	if err != nil {
		return nil, nil, err
	}

	data, err := s.sw.Download(art.StoragePath)
	if err != nil {
		return nil, nil, fmt.Errorf("seaweedfs download: %w", err)
	}

	return data, art, nil
}

// Delete removes the file from SeaweedFS and metadata from MongoDB.
// Idempotent: returns nil even if the artifact doesn't exist.
func (s *Storage) Delete(artifactID string) error {
	art, err := s.FindByID(artifactID)
	if err != nil {
		// Idempotent: not found is OK
		return nil
	}

	if err := s.sw.Delete(art.StoragePath); err != nil {
		return fmt.Errorf("seaweedfs delete: %w", err)
	}

	_, err = s.coll.DeleteOne(context.Background(), bson.M{"_id": artifactID})
	if err != nil {
		return fmt.Errorf("delete artifact metadata: %w", err)
	}
	return nil
}

// FindByID retrieves artifact metadata by ID.
func (s *Storage) FindByID(id string) (*artifact.Artifact, error) {
	var art artifact.Artifact
	err := s.coll.FindOne(context.Background(), bson.M{"_id": id}).Decode(&art)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("artifact %q not found", id)
		}
		return nil, fmt.Errorf("find artifact: %w", err)
	}
	return &art, nil
}

// ListBySession returns all artifacts for a session.
func (s *Storage) ListBySession(sessionID string) ([]artifact.Artifact, error) {
	cursor, err := s.coll.Find(context.Background(), bson.M{"session_id": sessionID})
	if err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}
	defer cursor.Close(context.Background())

	var artifacts []artifact.Artifact
	if err := cursor.All(context.Background(), &artifacts); err != nil {
		return nil, fmt.Errorf("decode artifacts: %w", err)
	}
	return artifacts, nil
}

// ListByTask returns all artifacts for a task.
func (s *Storage) ListByTask(taskID string) ([]artifact.Artifact, error) {
	cursor, err := s.coll.Find(context.Background(), bson.M{"task_id": taskID})
	if err != nil {
		return nil, fmt.Errorf("list task artifacts: %w", err)
	}
	defer cursor.Close(context.Background())

	var artifacts []artifact.Artifact
	if err := cursor.All(context.Background(), &artifacts); err != nil {
		return nil, fmt.Errorf("decode artifacts: %w", err)
	}
	return artifacts, nil
}
