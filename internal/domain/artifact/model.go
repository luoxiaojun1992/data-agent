package artifact

import (
	"time"

	"github.com/google/uuid"
)

// Artifact represents stored file metadata in MongoDB.
type Artifact struct {
	ID          string    `bson:"_id" json:"id"`
	UserID      string    `bson:"user_id" json:"user_id"`
	SessionID   string    `bson:"session_id" json:"session_id"`
	TaskID      string    `bson:"task_id,omitempty" json:"task_id,omitempty"`
	Name        string    `bson:"name" json:"name"`
	MimeType    string    `bson:"mime_type" json:"mime_type"`
	SizeBytes   int64     `bson:"size_bytes" json:"size_bytes"`
	StoragePath string    `bson:"storage_path" json:"storage_path"`
	Persistent  bool      `bson:"persistent" json:"persistent"`
	CreatedAt   time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time `bson:"updated_at" json:"updated_at"`
}

// NewArtifact creates a new Artifact with a generated ID.
func NewArtifact(userID, sessionID, taskID, name, mimeType, storagePath string, size int64, persistent bool) *Artifact {
	now := time.Now()
	return &Artifact{
		ID:          "artifact_" + uuid.New().String(),
		UserID:      userID,
		SessionID:   sessionID,
		TaskID:      taskID,
		Name:        name,
		MimeType:    mimeType,
		SizeBytes:   size,
		StoragePath: storagePath,
		Persistent:  persistent,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}
