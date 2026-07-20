package knowledge

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// EmbeddingFunc converts text into an embedding vector.
type EmbeddingFunc func(ctx context.Context, text string) ([]float32, error)

// Service handles knowledge base operations.
type Service struct {
	kb     repository.KBRepository
	vector repository.VectorRepository
	embed  EmbeddingFunc
	vecCol string
}

// NewService creates a knowledge base service.
func NewService(kb repository.KBRepository) *Service {
	return &Service{kb: kb, vecCol: "kb_chunks"}
}

// WithVectorIndex enables vector indexing.
func (s *Service) WithVectorIndex(repo repository.VectorRepository, embed EmbeddingFunc) *Service {
	s.vector = repo
	s.embed = embed
	return s
}

func (s *Service) CreateDoc(userID, title, fileName, fileType string, sizeBytes int64, gridFSFileID string) (*knowledge.KnowledgeDoc, error) {
	doc := &knowledge.KnowledgeDoc{
		ID:           "kbdoc_" + genShortID(),
		UserID:       userID,
		Title:        title,
		FileName:     fileName,
		FileType:     fileType,
		SizeBytes:    sizeBytes,
		GridFSFileID: gridFSFileID,
		Status:       knowledge.StatusUploaded,
	}
	if err := s.kb.CreateDoc(context.Background(), doc); err != nil {
		return nil, fmt.Errorf("insert knowledge doc: %w", err)
	}
	return doc, nil
}

func (s *Service) GetDoc(id string) (*knowledge.KnowledgeDoc, error) {
	return s.kb.GetDoc(context.Background(), id)
}

func (s *Service) DeleteDoc(id string) error {
	if err := s.kb.DeleteDoc(context.Background(), id); err != nil {
		return err
	}
	_, _ = s.kb.DeleteChunks(context.Background(), id)
	return nil
}

func (s *Service) ListDocs(userID string, skip, limit int64) ([]*knowledge.KnowledgeDoc, int64, error) {
	return s.kb.ListDocs(context.Background(), userID, skip, limit)
}

func (s *Service) ListAllDocs() ([]*knowledge.KnowledgeDoc, error) {
	return s.kb.ListAllDocs(context.Background())
}

func (s *Service) AddChunks(docID string, texts []string) error {
	var chunks []*knowledge.Chunk
	var vectors []repository.VectorPoint

	for _, text := range texts {
		chunk := &knowledge.Chunk{
			ID:     "chunk_" + uuid.New().String(),
			DocID:  docID,
			Content: text,
		}
		chunks = append(chunks, chunk)

		if s.embed != nil && s.vector != nil {
			vec, err := s.embed(context.Background(), text)
			if err == nil {
				vectors = append(vectors, repository.VectorPoint{
					ID:       chunk.ID,
					Vector:   vec,
					Metadata: map[string]interface{}{"doc_id": docID},
				})
			}
		}
	}

	if err := s.kb.AddChunks(context.Background(), chunks); err != nil {
		return fmt.Errorf("add chunks: %w", err)
	}

	if len(vectors) > 0 {
		if err := s.vector.Upsert(context.Background(), s.vecCol, vectors); err != nil {
			return fmt.Errorf("upsert vectors: %w", err)
		}
	}

	return s.kb.UpdateDocStatus(context.Background(), docID, knowledge.StatusIndexing, len(chunks))
}

func (s *Service) Search(query string, topK int) (*SearchResponse, error) {
	results := &SearchResponse{Results: []SearchResult{}}

	// Vector search
	if s.embed != nil && s.vector != nil {
		vec, err := s.embed(context.Background(), query)
		if err == nil {
			hits, err := s.vector.Search(context.Background(), s.vecCol, vec, topK, nil)
			if err == nil {
				for _, h := range hits {
					results.Results = append(results.Results, SearchResult{
						ChunkID: h.ID,
						Score:   float64(h.Score),
					})
				}
			}
		}
	}

	// Full-text fallback
	if len(results.Results) == 0 {
		textResults, err := s.kb.SearchChunks(context.Background(), query, topK)
		if err != nil {
			return nil, fmt.Errorf("search chunks: %w", err)
		}
		for _, r := range textResults {
			results.Results = append(results.Results, SearchResult{
				ChunkID: r.ChunkID,
				Text:    r.Content,
			})
		}
	}

	sort.Slice(results.Results, func(i, j int) bool {
		return results.Results[i].Score > results.Results[j].Score
	})

	return results, nil
}

type SearchResponse struct {
	Results []SearchResult `json:"results"`
}

type SearchResult struct {
	ChunkID string  `json:"chunk_id"`
	Text    string  `json:"text,omitempty"`
	Score   float64 `json:"score"`
}

// UploadFile uploads a file (kept for backward compat).
func (s *Service) UploadFile(userID, title, fileName, fileType string, sizeBytes int64, reader io.Reader) (*knowledge.KnowledgeDoc, error) {
	gridFSID := "fs_" + uuid.New().String()
	return s.CreateDoc(userID, title, fileName, fileType, sizeBytes, gridFSID)
}

func genShortID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())[:12]
}
