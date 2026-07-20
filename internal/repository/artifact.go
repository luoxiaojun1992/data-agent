package repository

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/artifact"
)

//go:generate mockery --name ArtifactRepository --output ./mocks --outpkg mocks

// ArtifactRepository defines the data access contract for agent artifacts.
type ArtifactRepository interface {
	Create(ctx context.Context, a *artifact.Artifact) error
	FindByID(ctx context.Context, id string) (*artifact.Artifact, error)
	Delete(ctx context.Context, id string) error
	ListBySession(ctx context.Context, sessionID string) ([]*artifact.Artifact, error)
	ListByTask(ctx context.Context, taskID string) ([]*artifact.Artifact, error)
}
