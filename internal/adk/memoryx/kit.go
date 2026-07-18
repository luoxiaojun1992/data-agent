package memoryx

import (
	"context"

	adkmemory "github.com/ieshan/adk-go-memory"
	"github.com/ieshan/adk-go-memory/adapter"
	"github.com/ieshan/adk-go-memory/compaction"
	"go.mongodb.org/mongo-driver/mongo"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/model"
)

// Kit wraps an adk-go-memory MemoryKit with the merge pipeline wired in.
type Kit struct {
	*adkmemory.MemoryKit
	storage *MongoStorage
	embed   func(ctx context.Context, text string) ([]float32, error)
}

// NewKit creates a new memory Kit backed by MongoDB.
func NewKit(db *mongo.Database, appName string, llm model.LLM, embedFn func(ctx context.Context, text string) ([]float32, error)) (*Kit, error) {
	storage := NewMongoStorage(db, appName)

	cfg := adkmemory.KitConfig{
		Storage:       storage,
		LLM:           llm,
		EmbeddingFunc: embedFn,
		Compaction: &compaction.Config{
			Strategy:  &compaction.SummarizationStrategy{LLM: llm},
			MaxEvents: 50,
		},
		DeltaMode: true,
	}

	memKit, err := adkmemory.New(cfg)
	if err != nil {
		return nil, err
	}

	return &Kit{
		MemoryKit: memKit,
		storage:   storage,
		embed:     embedFn,
	}, nil
}

// Service returns the memory.Service (implements google.golang.org/adk/memory.Service).
func (k *Kit) Service() memory.Service {
	return k.MemoryKit.Service
}

// Provider returns the adk-go-memory Provider for tool-level access.
func (k *Kit) Provider() *adkmemory.Provider {
	return k.MemoryKit.Provider
}

// SearchAndMerge searches for similar existing memories and performs cosine-merge
// if similarity ≥ mergeThreshold. The merged result is stored via the storage backend.
func (k *Kit) SearchAndMerge(ctx context.Context, obs []adapter.Observation) error {
	for i := range obs {
		o := &obs[i]
		if len(o.Embedding) == 0 && k.embed != nil && o.Content != "" {
			vec, err := k.embed(ctx, o.Content)
			if err == nil && len(vec) > 0 {
				o.Embedding = vec
			}
		}
		existing, err := k.storage.Search(ctx, &adapter.SearchOptions{
			Embedding:  o.Embedding,
			MaxResults: 1,
			UserID:     o.UserID,
			AppName:    o.AppName,
		})
		if err == nil && len(existing) > 0 {
			merged, _ := MergeSimilar(o, []*adapter.Observation{&existing[0].Observation}, k.embed)
			if merged != nil {
				o.Content = merged.Content
				o.Embedding = merged.Embedding
			}
		}
		if o.ID == [16]byte{} {
			o.ID = NewID()
		}
		if err := k.storage.Store(ctx, o); err != nil {
			return err
		}
	}
	return nil
}
