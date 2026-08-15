package fsops

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func newRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	return root
}

func TestWriteFile_CreatesFile(t *testing.T) {
	root := newRoot(t)
	if err := WriteFile(root, "a.txt", "hello\nworld\n"); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(root, "a.txt"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(b) != "hello\nworld\n" {
		t.Errorf("content = %q", string(b))
	}
}

func TestWriteFile_DoesNotCreateParent(t *testing.T) {
	root := newRoot(t)
	err := WriteFile(root, "sub/dir/a.txt", "x")
	if err == nil {
		t.Fatal("expected error when parent dir missing")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error = %v", err)
	}
}

func TestWriteFile_RejectsTraversal(t *testing.T) {
	root := newRoot(t)
	if err := WriteFile(root, "../escape.txt", "x"); err == nil {
		t.Fatal("expected path traversal error")
	}
	if err := WriteFile(root, "/etc/passwd", "x"); err == nil {
		t.Fatal("expected absolute path error")
	}
}

func TestMkdirAll_Recursive(t *testing.T) {
	root := newRoot(t)
	if err := MkdirAll(root, "a/b/c"); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	fi, err := os.Stat(filepath.Join(root, "a/b/c"))
	if err != nil || !fi.IsDir() {
		t.Fatalf("expected dir a/b/c, err=%v", err)
	}
}

func TestMkdirAll_Root(t *testing.T) {
	root := newRoot(t)
	if err := MkdirAll(root, ""); err != nil {
		t.Fatalf("MkdirAll root: %v", err)
	}
	if err := MkdirAll(root, "."); err != nil {
		t.Fatalf("MkdirAll '.': %v", err)
	}
}

func TestRemoveFile(t *testing.T) {
	root := newRoot(t)
	if err := WriteFile(root, "a.txt", "x"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveFile(root, "a.txt"); err != nil {
		t.Fatalf("RemoveFile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); !os.IsNotExist(err) {
		t.Errorf("file should be gone, err=%v", err)
	}
}

func TestRemoveFile_RefusesDir(t *testing.T) {
	root := newRoot(t)
	if err := MkdirAll(root, "d"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveFile(root, "d"); err == nil {
		t.Fatal("expected error deleting a dir as a file")
	}
}

func TestRemoveDir_Recursive(t *testing.T) {
	root := newRoot(t)
	if err := MkdirAll(root, "a/b"); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(root, "a/b/f.txt", "x"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveDir(root, "a"); err != nil {
		t.Fatalf("RemoveDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "a")); !os.IsNotExist(err) {
		t.Errorf("dir should be gone, err=%v", err)
	}
}

func TestRemoveDir_RefusesRoot(t *testing.T) {
	root := newRoot(t)
	if err := RemoveDir(root, ""); err == nil {
		t.Fatal("expected error deleting workspace root")
	}
	if err := RemoveDir(root, "."); err == nil {
		t.Fatal("expected error deleting workspace root ('.')")
	}
}

func TestReadFile_DefaultTenLines(t *testing.T) {
	root := newRoot(t)
	var sb strings.Builder
	for i := 1; i <= 20; i++ {
		sb.WriteString("line " + strconv.Itoa(i) + "\n")
	}
	if err := WriteFile(root, "f.txt", sb.String()); err != nil {
		t.Fatal(err)
	}
	res, err := ReadFile(root, "f.txt", 0, 0)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if res.TotalLines != 20 {
		t.Errorf("TotalLines = %d, want 20", res.TotalLines)
	}
	if res.StartLine != 1 || res.EndLine != 10 {
		t.Errorf("range = %d-%d, want 1-10", res.StartLine, res.EndLine)
	}
	if !strings.Contains(res.Content, "line 1\n") || strings.Contains(res.Content, "line 11") {
		t.Errorf("default content wrong: %q", res.Content)
	}
}

func TestReadFile_Range(t *testing.T) {
	root := newRoot(t)
	var sb strings.Builder
	for i := 1; i <= 20; i++ {
		sb.WriteString("line " + strconv.Itoa(i) + "\n")
	}
	if err := WriteFile(root, "f.txt", sb.String()); err != nil {
		t.Fatal(err)
	}
	res, err := ReadFile(root, "f.txt", 11, 15)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if res.StartLine != 11 || res.EndLine != 15 {
		t.Errorf("range = %d-%d, want 11-15", res.StartLine, res.EndLine)
	}
	if strings.Contains(res.Content, "line 10") || !strings.Contains(res.Content, "line 11") {
		t.Errorf("range content wrong: %q", res.Content)
	}
}

func TestReadFile_TooLargeRange(t *testing.T) {
	root := newRoot(t)
	var sb strings.Builder
	for i := 1; i <= 600; i++ {
		sb.WriteString("x\n")
	}
	if err := WriteFile(root, "f.txt", sb.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(root, "f.txt", 1, 600); err == nil {
		t.Fatal("expected error for oversized range")
	}
}

func TestReadFile_PastEnd(t *testing.T) {
	root := newRoot(t)
	if err := WriteFile(root, "f.txt", "a\nb\n"); err != nil {
		t.Fatal(err)
	}
	res, err := ReadFile(root, "f.txt", 1, 100)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if res.EndLine != 2 || res.TotalLines != 2 {
		t.Errorf("EndLine=%d TotalLines=%d, want 2/2", res.EndLine, res.TotalLines)
	}
}

func TestReadFile_RejectsDir(t *testing.T) {
	root := newRoot(t)
	if err := MkdirAll(root, "d"); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(root, "d", 0, 0); err == nil {
		t.Fatal("expected error reading a dir")
	}
}

func TestListDir_NonRecursive(t *testing.T) {
	root := newRoot(t)
	if err := MkdirAll(root, "sub"); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(root, "a.txt", "x"); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(root, "sub/inner.txt", "x"); err != nil {
		t.Fatal(err)
	}
	res, err := ListDir(root, "")
	if err != nil {
		t.Fatalf("ListDir: %v", err)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(res.Entries))
	}
	names := map[string]bool{}
	for _, e := range res.Entries {
		names[e.Name] = true
	}
	if !names["a.txt"] || !names["sub"] {
		t.Errorf("entries missing: %v", res.Entries)
	}
	// sub is dir, a.txt is file
	for _, e := range res.Entries {
		if e.Name == "sub" && !e.IsDir {
			t.Error("sub should be dir")
		}
		if e.Name == "a.txt" && e.IsDir {
			t.Error("a.txt should be file")
		}
	}
}

func TestListDir_RejectsFile(t *testing.T) {
	root := newRoot(t)
	if err := WriteFile(root, "f.txt", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := ListDir(root, "f.txt"); err == nil {
		t.Fatal("expected error listing a file")
	}
}
