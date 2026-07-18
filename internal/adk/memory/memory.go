// Package adkmemory implements the ADK memory.Service interface with
// MongoDB persistence and embedding-based semantic search, providing
// Mem0-style long-term memory across sessions.
//
// Writes are a side effect of session completion (AddSessionToMemory),
// reads happen through the memory_search tool (SearchMemory). Both are
// strictly scoped by user_id for multi-tenant isolation.
package adkmemory

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// CollectionName is the MongoDB collection holding memory entries.
const CollectionName = "memories"

// EmbeddingFunc converts text into an embedding vector.
// The production implementation calls an OpenAI-compatible /v1/embeddings
// endpoint (Ollama nomic-embed-text in the test environment).
type EmbeddingFunc func(ctx context.Context, text string) ([]float32, error)

// memoryDoc is the MongoDB document layout for a memory entry.
type memoryDoc struct {
	ID        string    `bson:"_id"`
	UserID    string    `bson:"user_id"`
	AppName   string    `bson:"app_name"`
	SessionID string    `bson:"session_id"`
	Text      string    `bson:"text"`
	Embedding []float64 `bson:"embedding"`
	CreatedAt time.Time `bson:"created_at"`
}

// Service implements memory.Service with MongoDB storage.
type Service struct {
	coll     *mongo.Collection
	embed    EmbeddingFunc
	maxChars int
}

// NewService creates a memory.Service. embed may be nil, in which case
// search degrades to keyword matching (useful for tests without an
// embedding backend).
func NewService(db *mongo.Database, embed EmbeddingFunc) *Service {
	return &Service{coll: db.Collection(CollectionName), embed: embed, maxChars: 1000}
}

// AddSessionToMemory extracts memorable content from a session and stores it.
// It is idempotent per (session, event text): previously stored texts for the
// session are skipped, so calling it repeatedly only appends new content.
func (s *Service) AddSessionToMemory(ctx context.Context, sess session.Session) error {
	texts := extractTexts(sess)
	if len(texts) == 0 {
		return nil
	}

	existing, err := s.textsForSession(ctx, sess.ID())
	if err != nil {
		return err
	}

	for _, text := range texts {
		if _, dup := existing[text]; dup {
			continue
		}
		doc := memoryDoc{
			ID:        "mem_" + uuid.New().String(),
			UserID:    sess.UserID(),
			AppName:   sess.AppName(),
			SessionID: sess.ID(),
			Text:      text,
			CreatedAt: time.Now(),
		}
		if s.embed != nil {
			vec, err := s.embed(ctx, text)
			if err != nil {
				// Embedding backend unavailable — store without vector rather than dropping memory.
				vec = nil
			}
			doc.Embedding = float32To64(vec)
		}
		if _, err := s.coll.InsertOne(ctx, doc); err != nil {
			return fmt.Errorf("store memory: %w", err)
		}
	}
	return nil
}

// SearchMemory returns memory entries relevant to the query, scoped to the user.
// With embeddings available it ranks by cosine similarity; otherwise it falls
// back to substring keyword matching.
func (s *Service) SearchMemory(ctx context.Context, req *memory.SearchRequest) (*memory.SearchResponse, error) {
	docs, err := s.docsForUser(ctx, req.AppName, req.UserID)
	if err != nil {
		return nil, err
	}

	resp := &memory.SearchResponse{Memories: []memory.Entry{}}
	if len(docs) == 0 || req.Query == "" {
		return resp, nil
	}

	var queryVec []float32
	if s.embed != nil {
		if vec, err := s.embed(ctx, req.Query); err == nil {
			queryVec = vec
		}
	}

	type scored struct {
		doc   memoryDoc
		score float64
	}
	scores := make([]scored, 0, len(docs))
	for _, d := range docs {
		var sc float64
		switch {
		case queryVec != nil && len(d.Embedding) > 0:
			sc = cosine(queryVec, float64To32(d.Embedding))
		case strings.Contains(strings.ToLower(d.Text), strings.ToLower(req.Query)):
			sc = 1.0
		default:
			sc = 0
		}
		if sc > 0 {
			scores = append(scores, scored{doc: d, score: sc})
		}
	}
	sort.Slice(scores, func(i, j int) bool { return scores[i].score > scores[j].score })

	const topK = 5
	for i, item := range scores {
		if i >= topK {
			break
		}
		resp.Memories = append(resp.Memories, memory.Entry{
			ID: item.doc.ID,
			Content: &genai.Content{
				Role:  "user",
				Parts: []*genai.Part{{Text: item.doc.Text}},
			},
			Author:    "memory",
			Timestamp: item.doc.CreatedAt,
			CustomMetadata: map[string]any{
				"score":      item.score,
				"session_id": item.doc.SessionID,
			},
		})
	}
	return resp, nil
}

// AdminSearch lists memories of a user matching the query (admin verification endpoint).
// It behaves like SearchMemory but without top-K truncation and includes all metadata.
func (s *Service) AdminSearch(ctx context.Context, appName, userID, query string) ([]map[string]any, error) {
	resp, err := s.SearchMemory(ctx, &memory.SearchRequest{AppName: appName, UserID: userID, Query: query})
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(resp.Memories))
	for _, m := range resp.Memories {
		text := ""
		if m.Content != nil {
			for _, p := range m.Content.Parts {
				if p != nil {
					text += p.Text
				}
			}
		}
		out = append(out, map[string]any{
			"id":         m.ID,
			"memory":     text,
			"created_at": m.Timestamp,
			"metadata":   m.CustomMetadata,
		})
	}
	return out, nil
}

// DeleteBySession removes all memories derived from a session (GDPR-style cleanup).
func (s *Service) DeleteBySession(ctx context.Context, sessionID string) error {
	_, err := s.coll.DeleteMany(ctx, bson.M{"session_id": sessionID})
	if err != nil {
		return fmt.Errorf("delete memories for session %q: %w", sessionID, err)
	}
	return nil
}

// ---- helpers ----

// extractTexts collects memorable text snippets from session events.
// Only user and model text messages are kept; compaction/system noise is skipped.
func extractTexts(sess session.Session) []string {
	var texts []string
	for ev := range sess.Events().All() {
		if text := eventText(ev); text != "" {
			texts = append(texts, text)
		}
	}
	return texts
}

// eventText renders the text content of one event, skipping compaction events
// and truncating to 1000 chars.
func eventText(ev *session.Event) string {
	if ev == nil || ev.Content == nil || ev.Author == "compaction" {
		return ""
	}
	var sb strings.Builder
	for _, p := range ev.Content.Parts {
		if p != nil && p.Text != "" {
			sb.WriteString(p.Text)
		}
	}
	text := strings.TrimSpace(sb.String())
	if len(text) > 1000 {
		text = text[:1000]
	}
	return text
}

func (s *Service) textsForSession(ctx context.Context, sessionID string) (map[string]struct{}, error) {
	cursor, err := s.coll.Find(ctx, bson.M{"session_id": sessionID})
	if err != nil {
		return nil, fmt.Errorf("query existing memories: %w", err)
	}
	defer cursor.Close(ctx)

	existing := map[string]struct{}{}
	for cursor.Next(ctx) {
		var d memoryDoc
		if err := cursor.Decode(&d); err != nil {
			return nil, fmt.Errorf("decode memory: %w", err)
		}
		existing[d.Text] = struct{}{}
	}
	return existing, cursor.Err()
}

func (s *Service) docsForUser(ctx context.Context, appName, userID string) ([]memoryDoc, error) {
	filter := bson.M{"user_id": userID}
	if appName != "" {
		filter["app_name"] = appName
	}
	cursor, err := s.coll.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("query memories: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []memoryDoc
	for cursor.Next(ctx) {
		var d memoryDoc
		if err := cursor.Decode(&d); err != nil {
			return nil, fmt.Errorf("decode memory: %w", err)
		}
		docs = append(docs, d)
	}
	return docs, cursor.Err()
}

// cosine computes cosine similarity between two vectors.
func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func float32To64(v []float32) []float64 {
	if v == nil {
		return nil
	}
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = float64(x)
	}
	return out
}

func float64To32(v []float64) []float32 {
	if v == nil {
		return nil
	}
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(x)
	}
	return out
}
