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
	// Qdrant requires non‑negative point IDs. FNV-64a can produce values
	// with the high bit set, so we clear the sign bit.
	return int64(h.Sum64() & (1<<63 - 1))
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
	results, err := v.client.Search(collection, vector, topK, filter)
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

// EnsureCollection creates the Qdrant collection if it doesn't exist.
// This is called at startup (like a migration) so KB documents can be
// indexed immediately without manual collection setup.
func (v *VectorStore) EnsureCollection(ctx context.Context, collection string, vectorSize int) error {
	exists, err := v.client.HasCollection(collection)
	if err != nil {
		return fmt.Errorf("qdrant check collection %s: %w", collection, err)
	}
	if exists {
		return nil
	}
	return v.client.CreateCollection(collection, vectorSize, "Cosine")
}

// DeleteCollection implements repository.VectorRepository.
func (v *VectorStore) DeleteCollection(ctx context.Context, collection string) error {
	// Qdrant client doesn't have a direct collection delete — no-op for now.
	return nil
}

// DeletePoints implements repository.VectorRepository. Idempotent: deleting
// zero points is not an error (SPEC-070 cascade delete).
func (v *VectorStore) DeletePoints(ctx context.Context, collection string, filter map[string]interface{}) error {
	return v.client.DeletePoints(collection, filter)
}

// GetChunkContents implements repository.VectorRepository. Chunk IDs are
// hashed to Qdrant point IDs; missing chunks are absent from the result.
func (v *VectorStore) GetChunkContents(ctx context.Context, collection string, chunkIDs []string) (map[string]map[string]interface{}, error) {
	ids := make([]int64, 0, len(chunkIDs))
	byID := make(map[int64]string, len(chunkIDs))
	for _, cid := range chunkIDs {
		id := stringToInt64(cid)
		ids = append(ids, id)
		byID[id] = cid
	}
	payloads, err := v.client.RetrievePoints(collection, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]map[string]interface{}, len(payloads))
	for id, p := range payloads {
		if cid, ok := byID[id]; ok {
			out[cid] = p
		}
	}
	return out, nil
}

// SetPayload updates the payload on all points matching a doc_id filter.
func (v *VectorStore) SetPayload(ctx context.Context, collection string, docID string, payload map[string]interface{}) error {
	filter := map[string]any{
		"must": []map[string]any{
			{"key": "doc_id", "match": map[string]any{"value": docID}},
		},
	}
	return v.client.SetPayload(collection, payload, filter)
}

var _ repository.VectorRepository = (*VectorStore)(nil)
