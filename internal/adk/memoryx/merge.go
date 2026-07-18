package memoryx

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/ieshan/adk-go-memory/adapter"
	"github.com/ieshan/idx"
)

const mergeThreshold = 0.92

// MergeSimilar checks whether `candidate` is semantically similar to any
// existing observation in `existing` (embedding cosine ≥ mergeThreshold).
// If a match is found, the existing observation's content is merged
// (longer+richer text kept) and its timestamp/embedding updated.
//
// Returns:
//
//	merged *adapter.Observation — the merged result (nil if no match)
//	matchedIdx int             — index in existing that matched (-1 if none)
func MergeSimilar(candidate *adapter.Observation, existing []*adapter.Observation, embed func(ctx context.Context, text string) ([]float32, error)) (*adapter.Observation, int) {
	if len(candidate.Embedding) == 0 || len(existing) == 0 {
		return nil, -1
	}

	for i, ex := range existing {
		if len(ex.Embedding) == 0 {
			continue
		}
		sim := cosine(candidate.Embedding, ex.Embedding)
		if sim >= mergeThreshold {
			merged := &adapter.Observation{
				ID:        ex.ID,
				Content:   mergeContent(ex.Content, candidate.Content),
				Level:     maxLevel(ex.Level, candidate.Level),
				SessionID: candidate.SessionID,
				UserID:    candidate.UserID,
				AppName:   candidate.AppName,
				Tags:      dedupeTags(append(ex.Tags, candidate.Tags...)),
				CreatedAt: ex.CreatedAt,
				Embedding: averageVectors(ex.Embedding, candidate.Embedding),
			}
			return merged, i
		}
	}
	return nil, -1
}

// cosine returns the cosine similarity between two float32 vectors.
func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func mergeContent(existing, candidate string) string {
	if len(candidate) > len(existing) {
		return candidate
	}
	if len(candidate) > len(existing)/2 && !strings.Contains(existing, candidate) {
		return existing + "; " + candidate
	}
	return existing
}

func maxLevel(a, b adapter.ObservationLevel) adapter.ObservationLevel {
	if a >= b {
		return a
	}
	return b
}

func dedupeTags(tags []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range tags {
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

func averageVectors(a, b []float32) []float32 {
	out := make([]float32, len(a))
	for i := range a {
		out[i] = (a[i] + b[i]) / 2
	}
	return out
}

// NewID generates a random idx.ID from the current time (simple nanosecond-based).
// Production use should prefer a proper UUID generator.
func NewID() idx.ID {
	now := time.Now().UnixNano()
	var id idx.ID
	for i := 0; i < 8 && i < 16; i++ {
		id[15-i] = byte(now >> (i * 8))
	}
	return id
}
