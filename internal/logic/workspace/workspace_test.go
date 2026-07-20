package workspace

import (
	"testing"

	"github.com/agiledragon/gomonkey/v2"

	"github.com/luoxiaojun1992/data-agent/internal/domain/artifact"
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

func TestSanitizePath_DoubleTraversal(t *testing.T) {
	// "../../../" → sanitize removes all ".." → "///" → TrimPrefix "/" → "//"
	got := sanitizePath("../../../")
	if got == "" || got == ".." || got == "/.." {
		t.Errorf("should not be empty or traversal: got %q", got)
	}
}

func TestSanitizePath_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"spaces in path", "my file.txt", "my file.txt"},
		{"unicode filename", "résumé.pdf", "résumé.pdf"},
		{"chinese characters", "报告文档.docx", "报告文档.docx"},
		{"very long path", "/a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s/t/u/v/w/x/y/z/file.txt", "a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s/t/u/v/w/x/y/z/file.txt"},
		{"repeated dot dot patterns", "../../../etc/../../passwd", "//etc///passwd"},
		{"single dot", ".", "."},
		{"dot slash", "./config", "./config"},
		{"mixed slashes", "a\\b/c\\d", "a/b/c/d"},
		{"leading slash only", "/", "unnamed"},
		{"only dots", "....", "unnamed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizePath(tt.input)
			if got != tt.want {
				t.Errorf("sanitizePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewManager(t *testing.T) {
	t.Run("nil storage", func(t *testing.T) {
		mgr := NewManager(nil)
		if mgr == nil {
			t.Error("NewManager should not return nil even with nil storage")
		}
	})

	t.Run("valid storage", func(t *testing.T) {
		storage := &artifactSvc.Storage{}
		mgr := NewManager(storage)
		if mgr == nil {
			t.Error("NewManager should return non-nil manager")
		}
	})
}

func TestReadFile(t *testing.T) {
	mockArt := &artifact.Artifact{
		ID:          "artifact_test123",
		Name:        "test.txt",
		StoragePath: "workspace/user/session/test.txt",
	}

	t.Run("successful read", func(t *testing.T) {
		storage := &artifactSvc.Storage{}
		mgr := NewManager(storage)

		patches := gomonkey.NewPatches()
		defer patches.Reset()
		patches.ApplyMethodReturn(storage, "FindByID", mockArt, nil)
		patches.ApplyMethodReturn(storage, "Download", []byte("hello world"), nil)

		data, err := mgr.ReadFile("user", "session", "test.txt")
		if err != nil {
			t.Fatalf("ReadFile unexpected error: %v", err)
		}
		if string(data) != "hello world" {
			t.Errorf("ReadFile: got %q, want %q", string(data), "hello world")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		storage := &artifactSvc.Storage{}
		mgr := NewManager(storage)

		patches := gomonkey.NewPatches()
		defer patches.Reset()
		patches.ApplyMethodReturn(storage, "FindByID", (*artifact.Artifact)(nil), errNotFound())

		_, err := mgr.ReadFile("user", "session", "nonexistent.txt")
		if err == nil {
			t.Error("expected error for non-existent file")
		}
	})
}

func TestWriteFile(t *testing.T) {
	mockArt := &artifact.Artifact{
		ID:          "artifact_write123",
		Name:        "output.txt",
		StoragePath: "workspace/user/session/output.txt",
	}

	t.Run("successful write", func(t *testing.T) {
		storage := &artifactSvc.Storage{}
		mgr := NewManager(storage)

		patches := gomonkey.NewPatches()
		defer patches.Reset()
		patches.ApplyMethodReturn(storage, "Upload", mockArt, nil)

		err := mgr.WriteFile("user", "session", "output.txt", []byte("test data"))
		if err != nil {
			t.Fatalf("WriteFile unexpected error: %v", err)
		}
	})

	t.Run("path sanitized in write", func(t *testing.T) {
		storage := &artifactSvc.Storage{}
		mgr := NewManager(storage)

		patches := gomonkey.NewPatches()
		defer patches.Reset()
		patches.ApplyMethodReturn(storage, "Upload", mockArt, nil)

		// Write with a path traversal filename — should be sanitized
		err := mgr.WriteFile("user", "session", "../../../etc/passwd", []byte("data"))
		if err != nil {
			t.Fatalf("WriteFile with traversal path unexpected error: %v", err)
		}
	})
}

// errNotFound returns a simple error for testing.
func errNotFound() error {
	return &simpleError{msg: "artifact not found"}
}

type simpleError struct {
	msg string
}

func (e *simpleError) Error() string {
	return e.msg
}

// ── Additional SanitizePath Edge Cases ──

func TestSanitizePath_BackslashesOnly(t *testing.T) {
	got := sanitizePath("a\\b\\c\\d")
	if got != "a/b/c/d" {
		t.Errorf("sanitizePath with backslashes: got %q, want a/b/c/d", got)
	}
}

func TestSanitizePath_MixedSlashes(t *testing.T) {
	got := sanitizePath("a\\b/c\\d/e\\f")
	if got != "a/b/c/d/e/f" {
		t.Errorf("sanitizePath with mixed slashes: got %q, want a/b/c/d/e/f", got)
	}
}

func TestSanitizePath_UnicodePath(t *testing.T) {
	got := sanitizePath("日本語/文件/报告.pdf")
	if got != "日本語/文件/报告.pdf" {
		t.Errorf("sanitizePath with unicode: got %q", got)
	}
}

func TestSanitizePath_AllEmptyAfterClean(t *testing.T) {
	got := sanitizePath(".")
	if got != "." {
		t.Errorf("sanitizePath of dot: got %q, want .", got)
	}

	got2 := sanitizePath("..")
	if got2 != "unnamed" {
		t.Errorf("sanitizePath of dotdot: got %q, want \"unnamed\"", got2)
	}
}

func TestSanitizePath_TabsAndSpecialChars(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"tab in path", "my\tfile.txt", "my\tfile.txt"},
		{"newline attempt", "file\n.txt", "file\n.txt"},
		{"percent encoded", "file%20name.txt", "file%20name.txt"},
		{"special chars", "file-name_123.txt", "file-name_123.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizePath(tt.input)
			if got != tt.want {
				t.Errorf("sanitizePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── ReadFile Error Paths ──

func TestReadFile_DownloadError(t *testing.T) {
	mockArt := &artifact.Artifact{
		ID:          "artifact_dl_err",
		Name:        "test.txt",
		StoragePath: "workspace/user/session/test.txt",
	}

	storage := &artifactSvc.Storage{}
	mgr := NewManager(storage)

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(storage, "FindByID", mockArt, nil)
	patches.ApplyMethodReturn(storage, "Download", []byte(nil), errDownload())

	_, err := mgr.ReadFile("user", "session", "test.txt")
	if err == nil {
		t.Error("expected error when download fails")
	}
}

func TestReadFile_PathSanitized(t *testing.T) {
	mockArt := &artifact.Artifact{
		ID:          "artifact_safe",
		Name:        "etc/passwd",
		StoragePath: "workspace/user/session/etc/passwd",
	}

	storage := &artifactSvc.Storage{}
	mgr := NewManager(storage)

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(storage, "FindByID", mockArt, nil)
	patches.ApplyMethodReturn(storage, "Download", []byte("safe data"), nil)

	// Path traversal in filename should be sanitized
	data, err := mgr.ReadFile("user", "session", "../../etc/passwd")
	if err != nil {
		t.Fatalf("ReadFile with sanitized path: %v", err)
	}
	if string(data) != "safe data" {
		t.Errorf("ReadFile: got %q, want 'safe data'", string(data))
	}
}

// ── WriteFile Error Path ──

func TestWriteFile_UploadError(t *testing.T) {
	storage := &artifactSvc.Storage{}
	mgr := NewManager(storage)

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(storage, "Upload", (*artifact.Artifact)(nil), errUpload())

	err := mgr.WriteFile("user", "session", "output.txt", []byte("data"))
	if err == nil {
		t.Error("expected error when upload fails")
	}
}

// ── List Tests ──

func TestList_Success(t *testing.T) {
	artifacts := []*artifact.Artifact{
		{Name: "file1.txt", StoragePath: "workspace/user1/sess1/file1.txt"},
		{Name: "file2.txt", StoragePath: "workspace/user1/sess1/file2.txt"},
		{Name: "other.txt", StoragePath: "workspace/user2/sess1/other.txt"},
	}

	storage := &artifactSvc.Storage{}
	mgr := NewManager(storage)

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(storage, "ListBySession", artifacts, nil)

	files, err := mgr.List("user1", "sess1")
	if err != nil {
		t.Fatalf("List unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("List: got %d files, want 2", len(files))
	}
	if files[0] != "file1.txt" {
		t.Errorf("List[0]: got %q, want file1.txt", files[0])
	}
	if files[1] != "file2.txt" {
		t.Errorf("List[1]: got %q, want file2.txt", files[1])
	}
}

func TestList_NoMatchingFiles(t *testing.T) {
	artifacts := []*artifact.Artifact{
		{Name: "other.txt", StoragePath: "workspace/other/sess1/other.txt"},
	}

	storage := &artifactSvc.Storage{}
	mgr := NewManager(storage)

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(storage, "ListBySession", artifacts, nil)

	files, err := mgr.List("user1", "sess1")
	if err != nil {
		t.Fatalf("List unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("List: got %d files, want 0", len(files))
	}
}

func TestList_EmptyList(t *testing.T) {
	storage := &artifactSvc.Storage{}
	mgr := NewManager(storage)

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(storage, "ListBySession", []*artifact.Artifact{}, nil)

	files, err := mgr.List("user1", "sess1")
	if err != nil {
		t.Fatalf("List unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("List: got %d files, want 0", len(files))
	}
}

func TestList_Error(t *testing.T) {
	storage := &artifactSvc.Storage{}
	mgr := NewManager(storage)

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(storage, "ListBySession", ([]*artifact.Artifact)(nil), errList())

	_, err := mgr.List("user1", "sess1")
	if err == nil {
		t.Error("expected error from ListBySession")
	}
}

// ── Cleanup Tests ──

func TestCleanup_Success(t *testing.T) {
	artifacts := []*artifact.Artifact{
		{ID: "id1", Name: "file1.txt"},
		{ID: "id2", Name: "file2.txt"},
	}

	storage := &artifactSvc.Storage{}
	mgr := NewManager(storage)

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(storage, "ListBySession", artifacts, nil)
	patches.ApplyMethodReturn(storage, "Delete", nil)

	err := mgr.Cleanup("sess1")
	if err != nil {
		t.Fatalf("Cleanup unexpected error: %v", err)
	}
}

func TestCleanup_EmptySession(t *testing.T) {
	storage := &artifactSvc.Storage{}
	mgr := NewManager(storage)

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(storage, "ListBySession", []*artifact.Artifact{}, nil)

	err := mgr.Cleanup("sess1")
	if err != nil {
		t.Fatalf("Cleanup of empty session: %v", err)
	}
}

func TestCleanup_ListError(t *testing.T) {
	storage := &artifactSvc.Storage{}
	mgr := NewManager(storage)

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(storage, "ListBySession", ([]*artifact.Artifact)(nil), errList())

	err := mgr.Cleanup("sess1")
	if err == nil {
		t.Error("expected error from ListBySession in Cleanup")
	}
}

func TestCleanup_DeleteError(t *testing.T) {
	artifacts := []*artifact.Artifact{
		{ID: "id1", Name: "file1.txt"},
	}

	storage := &artifactSvc.Storage{}
	mgr := NewManager(storage)

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(storage, "ListBySession", artifacts, nil)
	patches.ApplyMethodReturn(storage, "Delete", errDelete())

	err := mgr.Cleanup("sess1")
	if err == nil {
		t.Error("expected error when Delete fails in Cleanup")
	}
}

// ── NewManager with Non-Nil Storage ──

func TestNewManager_NonNilStorage(t *testing.T) {
	storage := &artifactSvc.Storage{}
	mgr := NewManager(storage)
	if mgr == nil {
		t.Fatal("NewManager with valid storage returned nil")
	}
}

// ── Error Helpers ──

func errDownload() error {
	return &simpleError{msg: "download failed"}
}

func errUpload() error {
	return &simpleError{msg: "upload failed"}
}

func errList() error {
	return &simpleError{msg: "list failed"}
}

func errDelete() error {
	return &simpleError{msg: "delete failed"}
}
