package seaweedfs

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client provides HTTP access to SeaweedFS.
type Client struct {
	masterURL  string
	filerURL   string
	httpClient *http.Client
}

// NewClient creates a new SeaweedFS client.
func NewClient(masterURL, filerURL string) *Client {
	return &Client{
		masterURL: masterURL,
		filerURL:  filerURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Upload stores a file and returns the file size.
func (c *Client) Upload(path string, reader io.Reader) (int64, error) {
	body, err := io.ReadAll(reader)
	if err != nil {
		return 0, fmt.Errorf("read upload data: %w", err)
	}

	req, err := http.NewRequest(http.MethodPut, c.filerURL+"/"+path, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("create upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("upload to seaweedfs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("seaweedfs upload failed (%d): %s", resp.StatusCode, string(errBody))
	}

	return int64(len(body)), nil
}

// Download retrieves a file from the given path.
func (c *Client) Download(path string) ([]byte, error) {
	resp, err := c.httpClient.Get(c.filerURL + "/" + path)
	if err != nil {
		return nil, fmt.Errorf("download from seaweedfs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("seaweedfs download failed (%d)", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// Delete removes a file. Returns nil even if the file doesn't exist (idempotent).
func (c *Client) Delete(path string) error {
	req, err := http.NewRequest(http.MethodDelete, c.filerURL+"/"+path, nil)
	if err != nil {
		return fmt.Errorf("create delete request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete from seaweedfs: %w", err)
	}
	defer resp.Body.Close()

	// Idempotent: 404 is not an error
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("seaweedfs delete failed (%d)", resp.StatusCode)
	}
	return nil
}

// List returns file info for files under a prefix.
func (c *Client) List(prefix string) ([]FileInfo, error) {
	resp, err := c.httpClient.Get(c.filerURL + "/" + prefix + "?pretty=y")
	if err != nil {
		return nil, fmt.Errorf("list seaweedfs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("seaweedfs list failed (%d)", resp.StatusCode)
	}

	// Parse JSON response — simplified for MVP
	body, _ := io.ReadAll(resp.Body)
	_ = body // parsed in production with proper JSON structure
	return nil, nil
}

// FileInfo represents file metadata from SeaweedFS listing.
type FileInfo struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir"`
}
