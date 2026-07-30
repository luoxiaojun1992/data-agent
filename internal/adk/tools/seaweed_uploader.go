package adktools

import (
	"context"
	"fmt"
	"os"

	"github.com/luoxiaojun1992/data-agent/internal/infra/seaweedfs"
)

// SeaweedFSUploader adapts seaweedfs.Client to the SeaweedUploader interface.
type SeaweedFSUploader struct {
	Client *seaweedfs.Client
}

func (u *SeaweedFSUploader) Upload(ctx context.Context, filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("stat: %w", err)
	}

	storagePath := fmt.Sprintf("artifacts/%d_%s", info.ModTime().UnixNano(), info.Name())
	_, err = u.Client.Upload(storagePath, file)
	if err != nil {
		return "", fmt.Errorf("seaweed upload: %w", err)
	}
	return storagePath, nil
}
