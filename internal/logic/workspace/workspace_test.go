package workspace

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/artifact"
	"github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
	artifactSvc "github.com/luoxiaojun1992/data-agent/internal/service/artifact"
)

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"normal.txt", "normal.txt"},
		{"../etc/passwd", "etc/passwd"},
		{"/root/file", "root/file"},
		{"a\\b\\c", "a/b/c"},
		{"", "unnamed"},
	}
	for _, tt := range tests {
		got := sanitizePath(tt.input)
		if got != tt.want {
			t.Errorf("sanitizePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestReadFile(t *testing.T) {
	mockFiles := mocks.NewFileRepository(t)
	mockMeta := mocks.NewArtifactRepository(t)
	storage := artifactSvc.NewStorage(mockFiles, mockMeta)
	mgr := NewManager(storage)

	storagePath := "workspace/user1/sess1/test.txt"
	art := &artifact.Artifact{ID: "art-id-1", StoragePath: storagePath}

	// ReadFile calls FindByID(storagePath), then Download(art.ID)
	// Download internally calls FindByID(art.ID) again, then files.Download(art.StoragePath)
	mockMeta.On("FindByID", mock.Anything, storagePath).Return(art, nil)
	mockMeta.On("FindByID", mock.Anything, "art-id-1").Return(art, nil)
	mockFiles.On("Download", mock.Anything, storagePath).Return([]byte("hello world"), nil)

	data, err := mgr.ReadFile("user1", "sess1", "test.txt")
	assert.NoError(t, err)
	assert.Equal(t, []byte("hello world"), data)
	mockMeta.AssertExpectations(t)
	mockFiles.AssertExpectations(t)
}

func TestReadFile_NotFound(t *testing.T) {
	mockFiles := mocks.NewFileRepository(t)
	mockMeta := mocks.NewArtifactRepository(t)
	storage := artifactSvc.NewStorage(mockFiles, mockMeta)
	mgr := NewManager(storage)

	mockMeta.On("FindByID", mock.MatchedBy(func(arg interface{}) bool {
		ctx, ok := arg.(context.Context)
		return ok && ctx != nil
	}), "workspace/user1/sess1/missing").Return((*artifact.Artifact)(nil), errors.New("not found"))

	_, err := mgr.ReadFile("user1", "sess1", "missing")
	assert.Error(t, err)
}
