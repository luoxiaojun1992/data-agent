package artifact

import (
	"time"

	"github.com/google/uuid"
)

// Artifact represents stored file metadata in MongoDB.
type Artifact struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	SessionID   string    `json:"session_id"`
	TaskID      string    `json:"task_id,omitempty"`
	Name        string    `json:"name"`
	MimeType    string    `json:"mime_type"`
	SizeBytes   int64     `json:"size_bytes"`
	StoragePath string    `json:"storage_path"`
	Persistent  bool      `json:"persistent"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
