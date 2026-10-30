package fsops

import (
	"os"
	"path/filepath"
	"testing"
)

// These tests cover the defensive error branches (empty/missing paths,
// wrong-type targets, out-of-range reads) that the happy-path tests in
// fsops_test.go do not reach.

func TestWriteFile_EmptyRel(t *testing.T) {
	root := newRoot(t)
	for _, rel := range []string{"", ".", "   "} {
		if err := WriteFile(root, rel, "x"); err == nil {
			t.Errorf("WriteFile(root, %q) expected error for empty path", rel)
		}
	}
}

func TestWriteFile_ParentNotDir(t *testing.T) {
	root := newRoot(t)
	if err := WriteFile(root, "f", "x"); err != nil {
		t.Fatal(err)
	}
	// "f" exists but is a file → parent is not a directory.
	if err := WriteFile(root, "f/a.txt", "x"); err == nil {
		t.Fatal("expected error when parent is a file")
	}
}

func TestRemoveFile_EmptyRel(t *testing.T) {
	root := newRoot(t)
	if err := RemoveFile(root, ""); err == nil {
		t.Error("expected error removing empty path as file")
	}
	if err := RemoveFile(root, "."); err == nil {
		t.Error("expected error removing '.' as file")
	}
}

func TestRemoveFile_NotExist(t *testing.T) {
	root := newRoot(t)
	if err := RemoveFile(root, "missing.txt"); err == nil {
		t.Fatal("expected error removing non-existent file")
	}
}

func TestRemoveDir_NotExist(t *testing.T) {
	root := newRoot(t)
	if err := RemoveDir(root, "missing"); err == nil {
		t.Fatal("expected error removing non-existent dir")
	}
}

func TestRemoveDir_RefusesFile(t *testing.T) {
	root := newRoot(t)
	if err := WriteFile(root, "f", "x"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveDir(root, "f"); err == nil {
		t.Fatal("expected error when target is a file, not a dir")
	}
}

func TestReadFile_EmptyRel(t *testing.T) {
	root := newRoot(t)
	if _, err := ReadFile(root, "", 0, 0); err == nil {
		t.Error("expected error reading empty path")
	}
}

func TestReadFile_NotExist(t *testing.T) {
	root := newRoot(t)
	if _, err := ReadFile(root, "missing.txt", 0, 0); err == nil {
		t.Fatal("expected error reading non-existent file")
	}
}

func TestReadFile_EndBeforeStart(t *testing.T) {
	root := newRoot(t)
	if err := WriteFile(root, "f.txt", "a\nb\nc\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(root, "f.txt", 3, 2); err == nil {
		t.Fatal("expected error when endLine < startLine")
	}
}

func TestReadFile_StartPastEnd(t *testing.T) {
	root := newRoot(t)
	if err := WriteFile(root, "f.txt", "a\nb\n"); err != nil {
		t.Fatal(err)
	}
	res, err := ReadFile(root, "f.txt", 5, 10)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if res.Content != "" {
		t.Errorf("content = %q, want empty when startLine past end", res.Content)
	}
	if res.TotalLines != 2 {
		t.Errorf("TotalLines = %d, want 2", res.TotalLines)
	}
}

func TestReadFile_TrailingNewlineDropped(t *testing.T) {
	root := newRoot(t)
	// exactly one line with a trailing newline → TotalLines should be 1.
	if err := WriteFile(root, "f.txt", "only\n"); err != nil {
		t.Fatal(err)
	}
	res, err := ReadFile(root, "f.txt", 0, 0)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if res.TotalLines != 1 {
		t.Errorf("TotalLines = %d, want 1", res.TotalLines)
	}
}

func TestListDir_NotExist(t *testing.T) {
	root := newRoot(t)
	if _, err := ListDir(root, "missing"); err == nil {
		t.Fatal("expected error listing non-existent dir")
	}
}

func TestResolve_AbsolutePath(t *testing.T) {
	root := newRoot(t)
	if _, err := resolve(root, string(os.PathSeparator)+"etc"+string(os.PathSeparator)+"passwd"); err == nil {
		t.Fatal("expected error for absolute path")
	}
}

func TestResolve_EmptyAndDot(t *testing.T) {
	root := newRoot(t)
	for _, rel := range []string{"", ".", "   "} {
		got, err := resolve(root, rel)
		if err != nil {
			t.Fatalf("resolve(%q): %v", rel, err)
		}
		if got != root {
			t.Errorf("resolve(%q) = %q, want root %q", rel, got, root)
		}
	}
}

func TestWriteFile_StatParentNonExistError(t *testing.T) {
	// os.Stat on a parent whose path is an existing file already covered by
	// TestWriteFile_ParentNotDir; this guards the empty-rel guard path returns
	// before any stat, keeping the workspace root protected.
	root := newRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(root, "sub/ok.txt", "x"); err != nil {
		t.Fatalf("WriteFile into existing dir: %v", err)
	}
}

func TestMkdirAll_PathTraversal(t *testing.T) {
	root := newRoot(t)
	if err := MkdirAll(root, ".."); err == nil {
		t.Fatal("expected error for '..' traversal")
	}
}

func TestMkdirAll_AbsolutePath(t *testing.T) {
	root := newRoot(t)
	if err := MkdirAll(root, string(os.PathSeparator)+"etc"); err == nil {
		t.Fatal("expected error for absolute path")
	}
}
