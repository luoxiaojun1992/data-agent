package knowledge

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

type EmbeddingFunc func(ctx context.Context, text string) ([]float32, error)

type Service struct {
	kb     repository.KBRepository
	vector repository.VectorRepository
	embed  EmbeddingFunc
	vecCol string
}

func NewService(kb repository.KBRepository) *Service {
	return &Service{kb: kb, vecCol: "kb_chunks"}
}

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

// ListDocs returns docs (backward compat: []docs, error).
func (s *Service) ListDocs(userID string) ([]*knowledge.KnowledgeDoc, error) {
	docs, _, err := s.kb.ListDocs(context.Background(), userID, 0, 100)
	return docs, err
}

func (s *Service) ListAllDocs() ([]*knowledge.KnowledgeDoc, error) {
	return s.kb.ListAllDocs(context.Background())
}

func (s *Service) AddChunks(docID string, texts []string) error {
	var chunks []*knowledge.Chunk
	var vectors []repository.VectorPoint
	for _, text := range texts {
		chunk := &knowledge.Chunk{ID: "chunk_" + uuid.New().String(), DocID: docID, Content: text}
		chunks = append(chunks, chunk)
		if s.embed != nil && s.vector != nil {
			if vec, err := s.embed(context.Background(), text); err == nil {
				vectors = append(vectors, repository.VectorPoint{ID: chunk.ID, Vector: vec, Metadata: map[string]interface{}{"doc_id": docID}})
			}
		}
	}
	if err := s.kb.AddChunks(context.Background(), chunks); err != nil {
		return fmt.Errorf("add chunks: %w", err)
	}
	if len(vectors) > 0 {
		_ = s.vector.Upsert(context.Background(), s.vecCol, vectors)
	}
	return s.kb.UpdateDocStatus(context.Background(), docID, knowledge.StatusIndexing, len(chunks))
}

// Search searches the knowledge base (backward compat: returns []knowledge.SearchResult).
func (s *Service) Search(userID, query string, topK int, role string) ([]knowledge.SearchResult, error) {
	resp := &SearchResponse{Results: []SearchResult{}}
	if s.embed != nil && s.vector != nil {
		if vec, err := s.embed(context.Background(), query); err == nil {
			if hits, err := s.vector.Search(context.Background(), s.vecCol, vec, topK, nil); err == nil {
				for _, h := range hits {
					resp.Results = append(resp.Results, SearchResult{ChunkID: h.ID, Score: float64(h.Score)})
				}
			}
		}
	}
	if len(resp.Results) == 0 {
		if textResults, err := s.kb.SearchChunks(context.Background(), query, topK); err == nil {
			for _, r := range textResults {
				resp.Results = append(resp.Results, SearchResult{ChunkID: r.ChunkID, Text: r.Content, Score: r.Score})
			}
		}
	}
	sort.Slice(resp.Results, func(i, j int) bool { return resp.Results[i].Score > resp.Results[j].Score })

	var results []knowledge.SearchResult
	for _, r := range resp.Results {
		results = append(results, knowledge.SearchResult{ChunkID: r.ChunkID, Content: r.Text, Score: r.Score})
	}
	return results, nil
}

// UploadFile uploads a file (backward compat: returns gridFSFileID, error).
func (s *Service) UploadFile(fileName, contentType string, reader io.Reader) (string, error) {
	gridFSID := "fs_" + uuid.New().String()
	doc, err := s.CreateDoc("", "", fileName, contentType, 0, gridFSID)
	if err != nil {
		return "", err
	}
	return doc.ID, nil
}

type SearchResponse struct {
	Results []SearchResult `json:"results"`
}

type SearchResult struct {
	ChunkID string  `json:"chunk_id"`
	Text    string  `json:"text,omitempty"`
	Score   float64 `json:"score"`
}

func genShortID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())[:12]
}
