package seaweedfs

import (
	"context"
	"io"

	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// FileStore implements repository.FileRepository backed by SeaweedFS.
type FileStore struct {
	client *Client
}

// NewFileStore creates a new FileStore.
func NewFileStore(client *Client) *FileStore {
	return &FileStore{client: client}
}

// Upload implements repository.FileRepository.
func (f *FileStore) Upload(ctx context.Context, path string, data []byte, contentType string) error {
	_, err := f.client.Upload(path, newBytesReader(data))
	return err
}

// Download implements repository.FileRepository.
func (f *FileStore) Download(ctx context.Context, path string) ([]byte, error) {
	reader, err := f.client.Download(path)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(reader)
}

// Delete implements repository.FileRepository.
func (f *FileStore) Delete(ctx context.Context, path string) error {
	return f.client.Delete(path)
}

var _ repository.FileRepository = (*FileStore)(nil)

type bytesReader struct {
	data   []byte
	offset int
}

func newBytesReader(data []byte) *bytesReader {
	return &bytesReader{data: data}
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}
