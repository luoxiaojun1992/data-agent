package workspace

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/artifact"
	"github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
	artifactSvc "github.com/luoxiaojun1992/data-agent/internal/service/artifact"
)

// newTestManager builds a Manager backed by mock repositories.
func newTestManager(t *testing.T) (*Manager, *mocks.FileRepository, *mocks.ArtifactRepository) {
	t.Helper()
	mockFiles := mocks.NewFileRepository(t)
	mockMeta := mocks.NewArtifactRepository(t)
	storage := artifactSvc.NewStorage(mockFiles, mockMeta)
	return NewManager(storage), mockFiles, mockMeta
}

// TestWriteFile_Success verifies that WriteFile uploads the file content and
// creates artifact metadata, returning no error.
func TestWriteFile_Success(t *testing.T) {
	mgr, mockFiles, mockMeta := newTestManager(t)

	var capturedName string
	mockFiles.On("Upload", mock.Anything, mock.Anything, []byte("hello"), "application/octet-stream").
		Run(func(args mock.Arguments) {
			capturedName = args.String(1) // storagePath passed to files.Upload
		}).
		Return(nil)
	mockMeta.On("Create", mock.Anything, mock.AnythingOfType("*artifact.Artifact")).Return(nil)

	err := mgr.WriteFile("user1", "sess1", "notes.txt", []byte("hello"))
	assert.NoError(t, err)
	// Storage.Upload builds the path from the sanitized filename; the file
	// repo must see the sanitized name in its storage path.
	assert.Contains(t, capturedName, "notes.txt")
	mockFiles.AssertExpectations(t)
	mockMeta.AssertExpectations(t)
}

// TestWriteFile_TraversalSanitized verifies that path-traversal input is
// cleaned before reaching the storage layer.
func TestWriteFile_TraversalSanitized(t *testing.T) {
	mgr, mockFiles, mockMeta := newTestManager(t)

	var capturedPath string
	mockFiles.On("Upload", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			capturedPath = args.String(1)
		}).
		Return(nil)
	mockMeta.On("Create", mock.Anything, mock.AnythingOfType("*artifact.Artifact")).Return(nil)

	err := mgr.WriteFile("u", "s", "../etc/passwd", []byte("x"))
	assert.NoError(t, err)
	// Sanitized filename must not contain "..".
	assert.NotContains(t, capturedPath, "..")
	assert.Contains(t, capturedPath, "etc/passwd")
}

// TestWriteFile_UploadError verifies that an upload error is wrapped and
// returned to the caller, and metadata creation is skipped.
func TestWriteFile_UploadError(t *testing.T) {
	mgr, mockFiles, mockMeta := newTestManager(t)

	mockFiles.On("Upload", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("disk full"))
	mockMeta.AssertNotCalled(t, "Create")

	err := mgr.WriteFile("u", "s", "f.txt", []byte("x"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "write workspace file")
	assert.Contains(t, err.Error(), "disk full")
}

// TestWriteFile_MetaCreateError verifies that when meta.Create fails, the
// file upload is rolled back via files.Delete.
func TestWriteFile_MetaCreateError(t *testing.T) {
	mgr, mockFiles, mockMeta := newTestManager(t)

	mockFiles.On("Upload", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockMeta.On("Create", mock.Anything, mock.AnythingOfType("*artifact.Artifact")).Return(errors.New("mongo down"))
	mockFiles.On("Delete", mock.Anything, mock.Anything).Return(nil)

	err := mgr.WriteFile("u", "s", "f.txt", []byte("x"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insert artifact metadata")
}

// TestList_Success verifies that List filters artifacts by the workspace
// prefix and returns only the matching file names.
func TestList_Success(t *testing.T) {
	mgr, _, mockMeta := newTestManager(t)

	arts := []*artifact.Artifact{
		{ID: "a1", Name: "alpha.txt", StoragePath: "workspace/user1/sess1/alpha.txt"},
		{ID: "a2", Name: "beta.txt", StoragePath: "workspace/user1/sess1/beta.txt"},
		// Different session prefix → must be filtered out.
		{ID: "a3", Name: "other.txt", StoragePath: "workspace/user1/other/other.txt"},
		// Non-workspace artifact → must be filtered out.
		{ID: "a4", Name: "task.bin", StoragePath: "artifacts/user1/sess1/task.bin"},
	}
	mockMeta.On("ListBySession", mock.Anything, "sess1").Return(arts, nil)

	files, err := mgr.List("user1", "sess1")
	assert.NoError(t, err)
	assert.Equal(t, []string{"alpha.txt", "beta.txt"}, files)
}

// TestList_Empty verifies that List returns an empty (non-nil) slice when no
// artifacts match the workspace prefix.
func TestList_Empty(t *testing.T) {
	mgr, _, mockMeta := newTestManager(t)

	mockMeta.On("ListBySession", mock.Anything, "sess1").Return([]*artifact.Artifact(nil), nil)

	files, err := mgr.List("user1", "sess1")
	assert.NoError(t, err)
	assert.Empty(t, files)
}

// TestList_RepoError verifies that List propagates repository errors.
func TestList_RepoError(t *testing.T) {
	mgr, _, mockMeta := newTestManager(t)

	mockMeta.On("ListBySession", mock.Anything, "sess1").Return([]*artifact.Artifact(nil), errors.New("db unavailable"))

	files, err := mgr.List("user1", "sess1")
	assert.Error(t, err)
	assert.Nil(t, files)
	assert.Contains(t, err.Error(), "db unavailable")
}

// TestCleanup_Success verifies that Cleanup deletes every artifact returned
// by ListBySession.
func TestCleanup_Success(t *testing.T) {
	mgr, mockFiles, mockMeta := newTestManager(t)

	arts := []*artifact.Artifact{
		{ID: "art-1", StoragePath: "workspace/u/s/f1.txt"},
		{ID: "art-2", StoragePath: "workspace/u/s/f2.txt"},
	}
	mockMeta.On("ListBySession", mock.Anything, "sess1").Return(arts, nil)
	// Storage.Delete calls meta.FindByID, meta.Delete, files.Delete for each.
	mockMeta.On("FindByID", mock.Anything, "art-1").Return(arts[0], nil)
	mockMeta.On("FindByID", mock.Anything, "art-2").Return(arts[1], nil)
	mockMeta.On("Delete", mock.Anything, "art-1").Return(nil)
	mockMeta.On("Delete", mock.Anything, "art-2").Return(nil)
	mockFiles.On("Delete", mock.Anything, "workspace/u/s/f1.txt").Return(nil)
	mockFiles.On("Delete", mock.Anything, "workspace/u/s/f2.txt").Return(nil)

	err := mgr.Cleanup("sess1")
	assert.NoError(t, err)
	mockMeta.AssertExpectations(t)
	mockFiles.AssertExpectations(t)
}

// TestCleanup_ListError verifies that Cleanup propagates ListBySession errors.
func TestCleanup_ListError(t *testing.T) {
	mgr, _, mockMeta := newTestManager(t)

	mockMeta.On("ListBySession", mock.Anything, "sess1").Return([]*artifact.Artifact(nil), errors.New("list failed"))

	err := mgr.Cleanup("sess1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "list failed")
}

// TestCleanup_DeleteError verifies that Cleanup wraps a per-artifact delete
// error with the offending artifact ID.
func TestCleanup_DeleteError(t *testing.T) {
	mgr, mockFiles, mockMeta := newTestManager(t)

	arts := []*artifact.Artifact{
		{ID: "art-1", StoragePath: "workspace/u/s/f1.txt"},
	}
	mockMeta.On("ListBySession", mock.Anything, "sess1").Return(arts, nil)
	// Storage.Delete first calls FindByID which fails.
	mockMeta.On("FindByID", mock.Anything, "art-1").Return((*artifact.Artifact)(nil), errors.New("find failed"))
	mockFiles.AssertNotCalled(t, "Delete")

	err := mgr.Cleanup("sess1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "art-1")
}

// TestCleanup_MetaDeleteError verifies that Cleanup wraps a metadata-delete
// failure with the artifact ID.
func TestCleanup_MetaDeleteError(t *testing.T) {
	mgr, mockFiles, mockMeta := newTestManager(t)

	arts := []*artifact.Artifact{
		{ID: "art-9", StoragePath: "workspace/u/s/f9.txt"},
	}
	mockMeta.On("ListBySession", mock.Anything, "sess1").Return(arts, nil)
	mockMeta.On("FindByID", mock.Anything, "art-9").Return(arts[0], nil)
	mockMeta.On("Delete", mock.Anything, "art-9").Return(errors.New("meta delete failed"))
	mockFiles.AssertNotCalled(t, "Delete")

	err := mgr.Cleanup("sess1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "art-9")
}

// TestReadFile_DownloadError covers the branch where Download fails after a
// successful FindByID lookup.
func TestReadFile_DownloadError(t *testing.T) {
	mgr, mockFiles, mockMeta := newTestManager(t)

	storagePath := "workspace/u/s/file.txt"
	art := &artifact.Artifact{ID: "art-x", StoragePath: storagePath}

	mockMeta.On("FindByID", mock.Anything, storagePath).Return(art, nil)
	mockMeta.On("FindByID", mock.Anything, "art-x").Return(art, nil)
	mockFiles.On("Download", mock.Anything, storagePath).Return(([]byte)(nil), errors.New("s3 down"))

	data, err := mgr.ReadFile("u", "s", "file.txt")
	assert.Error(t, err)
	assert.Nil(t, data)
}

// TestNewManager verifies the constructor wires the storage dependency.
func TestNewManager(t *testing.T) {
	mgr, _, _ := newTestManager(t)
	assert.NotNil(t, mgr)
	assert.NotNil(t, mgr.storage)
}

// TestSanitizePath_AdditionalCases covers traversal-only and backslash inputs
// beyond the cases in the existing test file.
func TestSanitizePath_AdditionalCases(t *testing.T) {
	assert.Equal(t, "unnamed", sanitizePath("/"))           // TrimPrefix("/") → "" → "unnamed"
	assert.NotContains(t, sanitizePath("../.."), "..")      // no traversal chars remain
	assert.Contains(t, sanitizePath("a\\b\\c"), "a/b/c")    // backslashes converted
	assert.Equal(t, "etc/passwd", sanitizePath("../etc/passwd"))
}
