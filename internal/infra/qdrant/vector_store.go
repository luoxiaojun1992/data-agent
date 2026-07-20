package qdrant

import (
	"context"
	"fmt"
	"hash/fnv"

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

func stringToInt64(s string) int64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return int64(h.Sum64())
}

func int64ToString(i int64) string {
	return fmt.Sprintf("%d", i)
}

// Upsert implements repository.VectorRepository.
func (v *VectorStore) Upsert(ctx context.Context, collection string, vectors []repository.VectorPoint) error {
	points := make([]Point, len(vectors))
	for i, vp := range vectors {
		points[i] = Point{
			ID:      stringToInt64(vp.ID),
			Vector:  vp.Vector,
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
			ID:       int64ToString(r.ID),
			Score:    r.Score,
			Metadata: r.Payload,
		}
	}
	return hits, nil
}

// DeleteCollection implements repository.VectorRepository.
func (v *VectorStore) DeleteCollection(ctx context.Context, collection string) error {
	// Qdrant client doesn't have a direct collection delete — no-op for now.
	return nil
}

var _ repository.VectorRepository = (*VectorStore)(nil)
