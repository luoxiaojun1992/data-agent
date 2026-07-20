package qdrant

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// VectorStore implements repository.VectorRepository backed by Qdrant.
type VectorStore struct {
	client *Client
}

// NewVectorStore creates a new VectorStore.
func NewVectorStore(client *Client) *VectorStore {
	return &VectorStore{client: client}
}

// Upsert implements repository.VectorRepository.
func (v *VectorStore) Upsert(ctx context.Context, collection string, vectors []repository.VectorPoint) error {
	points := make([]Point, len(vectors))
	for i, vp := range vectors {
		points[i] = Point{
			ID:     vp.ID,
			Vector: vp.Vector,
			Payload: vp.Metadata,
		}
	}
	return v.client.UpsertPoints(collection, points)
}

// Search implements repository.VectorRepository.
func (v *VectorStore) Search(ctx context.Context, collection string, vector []float32, topK int, filter map[string]interface{}) ([]repository.VectorSearchHit, error) {
	results, err := v.client.Search(collection, vector, topK)
	if err != nil {
		return nil, err
	}
	hits := make([]repository.VectorSearchHit, len(results))
	for i, r := range results {
		hits[i] = repository.VectorSearchHit{
			ID:       r.ID,
			Score:    r.Score,
			Metadata: r.Payload,
		}
	}
	return hits, nil
}

// DeleteCollection implements repository.VectorRepository.
func (v *VectorStore) DeleteCollection(ctx context.Context, collection string) error {
	return v.client.DeleteCollection(collection)
}

var _ repository.VectorRepository = (*VectorStore)(nil)
