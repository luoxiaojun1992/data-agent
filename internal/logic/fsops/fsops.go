// Package fsops implements session-workspace-scoped file system operations
// for the data-agent skills (file_write, dir_create, file_delete, dir_delete,
// file_read, dir_list).
//
// Every operation resolves a session-relative path against a workspace root
// and refuses anything that escapes that root (absolute paths, "..").
// The caller (tools.go) passes the session workspace as root and the LLM only
// ever supplies a relative path — it never sees or supplies the root itself.
package fsops

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxReadLines caps a single file_read call. Ranges larger than this must be
// read in multiple calls, which prevents the LLM from slurping an entire
// (possibly huge) file in one shot.
const maxReadLines = 500

// resolve maps a session-relative path onto the workspace root and rejects
// anything that would escape the root. An empty or "." path resolves to the
// root itself.
func resolve(root, rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "." {
		return root, nil
	}
	clean := filepath.Clean(rel)
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("path must be relative to the session workspace")
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path traversal denied: %q", rel)
	}
	return filepath.Join(root, clean), nil
}

// WriteFile writes content to a session-relative file. It does NOT create
// missing parent directories — the parent must already exist.
func WriteFile(root, rel, content string) error {
	if strings.TrimSpace(rel) == "" || rel == "." {
		return fmt.Errorf("path must name a file, not the workspace root")
	}
	full, err := resolve(root, rel)
	if err != nil {
		return err
	}
	parent := filepath.Dir(full)
	info, err := os.Stat(parent)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("parent directory does not exist: %s", rel)
		}
		return fmt.Errorf("stat parent: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("parent is not a directory: %s", rel)
	}
	return os.WriteFile(full, []byte(content), 0o644)
}

// MkdirAll creates a session-relative directory recursively.
func MkdirAll(root, rel string) error {
	full, err := resolve(root, rel)
	if err != nil {
		return err
	}
	if full == root {
		return nil // workspace root already exists
	}
	return os.MkdirAll(full, 0o700)
}

// RemoveFile deletes a session-relative file. Directories are refused.
func RemoveFile(root, rel string) error {
	if strings.TrimSpace(rel) == "" || rel == "." {
		return fmt.Errorf("cannot delete the workspace root as a file")
	}
	full, err := resolve(root, rel)
	if err != nil {
		return err
	}
	info, err := os.Stat(full)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory, use dir_delete instead", rel)
	}
	return os.Remove(full)
}

// RemoveDir recursively deletes a session-relative directory (children
// included). The workspace root itself cannot be deleted.
func RemoveDir(root, rel string) error {
	if strings.TrimSpace(rel) == "" || rel == "." {
		return fmt.Errorf("cannot delete the workspace root")
	}
	full, err := resolve(root, rel)
	if err != nil {
		return err
	}
	info, err := os.Stat(full)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", rel)
	}
	return os.RemoveAll(full)
}

// ReadResult is the outcome of ReadFile.
type ReadResult struct {
	Path       string `json:"path"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	TotalLines int    `json:"total_lines"`
	Content    string `json:"content"`
}

// ReadFile reads a line range from a session-relative file (1-based,
// inclusive). Without a range it returns the first 10 lines. startLine and
// endLine of 0 mean "unspecified": startLine defaults to 1 and endLine
// defaults to startLine+9 (a 10-line window).
func ReadFile(root, rel string, startLine, endLine int) (ReadResult, error) {
	if strings.TrimSpace(rel) == "" || rel == "." {
		return ReadResult{}, fmt.Errorf("path must name a file, not the workspace root")
	}
	full, err := resolve(root, rel)
	if err != nil {
		return ReadResult{}, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return ReadResult{}, err
	}
	if info.IsDir() {
		return ReadResult{}, fmt.Errorf("%s is a directory, use dir_list instead", rel)
	}

	data, err := os.ReadFile(full)
	if err != nil {
		return ReadResult{}, err
	}
	lines := strings.Split(string(data), "\n")
	// Drop a trailing empty element caused by a final newline.
	total := len(lines)
	if total > 0 && lines[total-1] == "" {
		total--
		lines = lines[:total]
	}

	if startLine <= 0 {
		startLine = 1
	}
	if endLine <= 0 {
		endLine = startLine + 9
	}
	if endLine < startLine {
		return ReadResult{}, fmt.Errorf("end_line (%d) must be >= start_line (%d)", endLine, startLine)
	}
	if endLine-startLine+1 > maxReadLines {
		return ReadResult{}, fmt.Errorf("requested range too large (max %d lines per call, use multiple calls)", maxReadLines)
	}

	if startLine > total {
		return ReadResult{
			Path:       rel,
			StartLine:  startLine,
			EndLine:    endLine,
			TotalLines: total,
			Content:    "",
		}, nil
	}
	if endLine > total {
		endLine = total
	}
	// lines is 0-indexed; startLine/endLine are 1-based.
	selected := lines[startLine-1 : endLine]
	return ReadResult{
		Path:       rel,
		StartLine:  startLine,
		EndLine:    endLine,
		TotalLines: total,
		Content:    strings.Join(selected, "\n"),
	}, nil
}

// Entry describes one item returned by ListDir (non-recursive).
type Entry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size,omitempty"`
}

// ListResult is the outcome of ListDir.
type ListResult struct {
	Path    string  `json:"path"`
	Entries []Entry `json:"entries"`
}

// ListDir lists the immediate children of a session-relative directory
// (non-recursive). An empty path lists the workspace root.
func ListDir(root, rel string) (ListResult, error) {
	full, err := resolve(root, rel)
	if err != nil {
		return ListResult{}, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return ListResult{}, err
	}
	if !info.IsDir() {
		return ListResult{}, fmt.Errorf("%s is not a directory, use file_read instead", rel)
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return ListResult{}, err
	}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		entry := Entry{Name: e.Name(), IsDir: e.IsDir()}
		if !e.IsDir() {
			if fi, err := e.Info(); err == nil {
				entry.Size = fi.Size()
			}
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return out[i].Name < out[j].Name
	})
	return ListResult{Path: rel, Entries: out}, nil
}
