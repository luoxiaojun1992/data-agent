package artifact

import (
	"strings"
	"testing"
)

func TestNewArtifact(t *testing.T) {
	userID := "user-1"
	sessionID := "sess-1"
	name := "report.pdf"
	mimeType := "application/pdf"
	storagePath := "/storage/report.pdf"
	size := int64(1024)

	t.Run("basic creation", func(t *testing.T) {
		a := NewArtifact(userID, sessionID, "", name, mimeType, storagePath, size, false)

		if !strings.HasPrefix(a.ID, "artifact_") {
			t.Errorf("ID should start with 'artifact_': got %s", a.ID)
		}
		if a.UserID != userID {
			t.Errorf("UserID: got %s, want %s", a.UserID, userID)
		}
		if a.Name != name {
			t.Errorf("Name: got %s, want %s", a.Name, name)
		}
		if a.SizeBytes != size {
			t.Errorf("SizeBytes: got %d, want %d", a.SizeBytes, size)
		}
	})

	t.Run("created equals updated", func(t *testing.T) {
		a := NewArtifact(userID, sessionID, "", name, mimeType, storagePath, size, false)
		if !a.CreatedAt.Equal(a.UpdatedAt) {
			t.Error("CreatedAt should equal UpdatedAt")
		}
	})

	t.Run("taskID can be empty", func(t *testing.T) {
		a := NewArtifact(userID, sessionID, "", name, mimeType, storagePath, size, false)
		if a.TaskID != "" {
			t.Errorf("TaskID should be empty: got %s", a.TaskID)
		}
	})

	t.Run("taskID set correctly", func(t *testing.T) {
		taskID := "task-123"
		a := NewArtifact(userID, sessionID, taskID, name, mimeType, storagePath, size, false)
		if a.TaskID != taskID {
			t.Errorf("TaskID: got %s, want %s", a.TaskID, taskID)
		}
	})

	t.Run("persistent flag", func(t *testing.T) {
		a := NewArtifact(userID, sessionID, "", name, mimeType, storagePath, size, true)
		if !a.Persistent {
			t.Error("Persistent should be true")
		}
	})

	t.Run("unique IDs", func(t *testing.T) {
		a1 := NewArtifact(userID, sessionID, "", name, mimeType, storagePath, size, false)
		a2 := NewArtifact(userID, sessionID, "", name, mimeType, storagePath, size, false)
		if a1.ID == a2.ID {
			t.Error("two NewArtifact calls should produce unique IDs")
		}
	})
}
