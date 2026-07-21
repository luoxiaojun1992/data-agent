package workspace

import (
	"fmt"
	"strings"

	"github.com/luoxiaojun1992/data-agent/internal/service/artifact"
)

// Manager handles per-session workspace file isolation.
type Manager struct {
	storage *artifact.Storage
}

// NewManager creates a workspace manager.
func NewManager(storage *artifact.Storage) *Manager {
	return &Manager{storage: storage}
}

// ReadFile reads a file from the session's workspace.
func (m *Manager) ReadFile(userID, sessionID, filename string) ([]byte, error) {
	safePath := sanitizePath(filename)
	storagePath := fmt.Sprintf("workspace/%s/%s/%s", userID, sessionID, safePath)
	art, err := m.storage.FindByID(storagePath)
	if err != nil {
		return nil, fmt.Errorf("workspace file not found: %w", err)
	}
	data, err := m.storage.Download(art.ID)
	return data, err
}

// WriteFile writes a file to the session's workspace.
func (m *Manager) WriteFile(userID, sessionID, filename string, data []byte) error {
	safePath := sanitizePath(filename)

	_, err := m.storage.Upload(
		userID, sessionID, "",
		safePath, "application/octet-stream",
		strings.NewReader(string(data)),
		false, // workspace files are temporary
	)
	if err != nil {
		return fmt.Errorf("write workspace file: %w", err)
	}
	return nil
}

// List returns files in the session's workspace.
func (m *Manager) List(userID, sessionID string) ([]string, error) {
	prefix := fmt.Sprintf("workspace/%s/%s/", userID, sessionID)
	artifacts, err := m.storage.ListBySession(sessionID)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, a := range artifacts {
		if strings.HasPrefix(a.StoragePath, prefix) {
			files = append(files, a.Name)
		}
	}
	return files, nil
}

// Cleanup removes all files in a session's workspace.
func (m *Manager) Cleanup(sessionID string) error {
	artifacts, err := m.storage.ListBySession(sessionID)
	if err != nil {
		return err
	}
	for _, a := range artifacts {
		if err := m.storage.Delete(a.ID); err != nil {
			return fmt.Errorf("cleanup artifact %q: %w", a.ID, err)
		}
	}
	return nil
}

// sanitizePath prevents path traversal attacks.
func sanitizePath(path string) string {
	// Remove any directory traversal attempts
	cleaned := strings.ReplaceAll(path, "..", "")
	cleaned = strings.ReplaceAll(cleaned, "\\", "/")
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "" {
		return "unnamed"
	}
	return cleaned
}
