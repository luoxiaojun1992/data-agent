package knowledge

import (
	"context"
	"fmt"
	"io"
	"log"
	"math"
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
	now := time.Now()
	doc := &knowledge.KnowledgeDoc{
		ID:           "kbdoc_" + genShortID(),
		UserID:       userID,
		Title:        title,
		FileName:     fileName,
		FileType:     fileType,
		SizeBytes:    sizeBytes,
		GridFSFileID: gridFSFileID,
		CreatedAt:    now,
		UpdatedAt:    now,
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

// ListDocs returns paginated docs for a user (own docs only, backward compat).
func (s *Service) ListDocs(userID string, page, pageSize int) ([]*knowledge.KnowledgeDoc, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	skip := int64((page - 1) * pageSize)
	return s.kb.ListDocs(context.Background(), userID, skip, int64(pageSize))
}

// ListDocsByVisibility returns docs visible to the user based on role.
func (s *Service) ListDocsByVisibility(userID string, isSystemAdmin bool, page, pageSize int) ([]*knowledge.KnowledgeDoc, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	skip := int64((page - 1) * pageSize)
	return s.kb.ListDocsByVisibility(context.Background(), userID, isSystemAdmin, skip, int64(pageSize))
}

// ListAllDocs returns paginated docs globally (admin).
func (s *Service) ListAllDocs(page, pageSize int) ([]*knowledge.KnowledgeDoc, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	skip := int64((page - 1) * pageSize)
	return s.kb.ListAllDocs(context.Background(), skip, int64(pageSize))
}

func (s *Service) AddChunks(docID string, texts []string) error {
	// Look up parent doc to propagate creator_id and is_public to chunks.
	doc, err := s.kb.GetDoc(context.Background(), docID)
	if err != nil {
		return fmt.Errorf("get doc: %w", err)
	}
	var chunks []*knowledge.Chunk
	var vectors []repository.VectorPoint
	for idx, text := range texts {
		chunk := &knowledge.Chunk{
			ID:        "chunk_" + uuid.New().String(),
			DocID:     docID,
			CreatorID: doc.UserID,
			IsPublic:  doc.IsPublic,
			Content:   text,
			ChunkIdx:  idx,
			CharCount: len([]rune(text)),
		}
		chunks = append(chunks, chunk)
		if s.embed != nil && s.vector != nil {
			vec, err := s.embed(context.Background(), text)
			if err != nil {
				log.Printf("[kb] embed failed for chunk=%s: %v", chunk.ID, err)
				continue
			}
			if vec == nil {
				log.Printf("[kb] embed returned nil for chunk=%s (check embedding config)", chunk.ID)
				continue
			}
			vectors = append(vectors, repository.VectorPoint{
				ID:     chunk.ID,
				Vector: vec,
				Metadata: map[string]interface{}{
					"doc_id":     docID,
					"content":    text,
					"creator_id": doc.UserID,
					"is_public":  doc.IsPublic,
				},
			})
		}
	}
	if err := s.kb.AddChunks(context.Background(), chunks); err != nil {
		return fmt.Errorf("add chunks: %w", err)
	}
	if len(vectors) > 0 {
		if err := s.vector.Upsert(context.Background(), s.vecCol, vectors); err != nil {
			log.Printf("[kb] upsert vectors to Qdrant failed: %v", err)
		}
	}
	return s.kb.UpdateDocStatus(context.Background(), docID, knowledge.StatusIndexing, len(chunks), 0)
}

// Search searches the knowledge base using vector + text fallback, with
// permission filtering. System admin sees all; regular users see own docs + public.
func (s *Service) Search(userID, query string, topK int, isSystemAdmin bool) ([]knowledge.SearchResult, error) {
	results := s.vectorSearch(query, topK, userID, isSystemAdmin)
	if len(results) == 0 {
		results = s.textSearch(query, topK, userID, isSystemAdmin)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	return results, nil
}

func (s *Service) vectorSearch(query string, topK int, userID string, isSystemAdmin bool) []knowledge.SearchResult {
	if s.embed == nil || s.vector == nil {
		return nil
	}
	vec, err := s.embed(context.Background(), query)
	if err != nil {
		return nil
	}
	// Apply permission filter to vector search
	var filter map[string]interface{}
	if !isSystemAdmin {
		filter = map[string]interface{}{
			"must": []interface{}{
				map[string]interface{}{
					"should": []interface{}{
						map[string]interface{}{"key": "creator_id", "match": map[string]interface{}{"value": userID}},
						map[string]interface{}{"key": "is_public", "match": map[string]interface{}{"value": true}},
					},
				},
			},
		}
	}
	hits, err := s.vector.Search(context.Background(), s.vecCol, vec, topK, filter)
	if err != nil {
		return nil
	}
	var results []knowledge.SearchResult
	for _, h := range hits {
		docID, _ := h.Metadata["doc_id"].(string)
		content, _ := h.Metadata["content"].(string)
		results = append(results, knowledge.SearchResult{ChunkID: h.ID, DocID: docID, Content: content, Score: float64(h.Score), Source: "qdrant"})
	}
	return results
}

func (s *Service) textSearch(query string, topK int, userID string, isSystemAdmin bool) []knowledge.SearchResult {
	textResults, err := s.kb.SearchChunks(context.Background(), query, userID, isSystemAdmin, topK)
	if err != nil {
		return nil
	}
	var results []knowledge.SearchResult
	for _, r := range textResults {
		results = append(results, knowledge.SearchResult{ChunkID: r.ChunkID, Content: r.Content, Score: r.Score})
	}
	return results
}

// UploadFile uploads a file to GridFS and returns the gridFSFileID.
func (s *Service) UploadFile(fileName, contentType string, reader io.Reader) (string, error) {
	gridFSID := "kbdoc_" + genShortID()
	if err := s.kb.UploadFile(context.Background(), gridFSID, reader); err != nil {
		return "", fmt.Errorf("gridfs upload: %w", err)
	}
	return gridFSID, nil
}

// IndexDocument performs the full indexing pipeline on a knowledge document:
// GridFS chunked download → sentence boundary splitting → LLM semantic chunking
// → Embedding → Qdrant + MongoDB write. llmChunkFn is called for each
// sentence-delimited segment to produce semantic chunks (handler-driven).
func (s *Service) IndexDocument(ctx context.Context, docID string, llmChunkFn func(text string) ([]string, error)) error {
	doc, err := s.kb.GetDoc(ctx, docID)
	if err != nil {
		return fmt.Errorf("get doc %s: %w", docID, err)
	}

	// 1. Download file from GridFS
	data, err := s.kb.DownloadFile(ctx, doc.GridFSFileID)
	if err != nil {
		return fmt.Errorf("download file %s: %w", doc.GridFSFileID, err)
	}

	// 2. Split into sentence-boundary segments via fixed-window reading.
	//    Avoids loading giant files entirely into memory for LLM chunking.
	segments := splitBySentence(string(data), chunkWindowSize)

	// 3. LLM semantic chunking per segment
	var allChunks []string
	for _, seg := range segments {
		chunks, err := llmChunkFn(seg)
		if err != nil {
			return fmt.Errorf("llm chunk: %w", err)
		}
		allChunks = append(allChunks, chunks...)
	}
	if len(allChunks) == 0 {
		return fmt.Errorf("no chunks produced for doc %s", docID)
	}

	// 4. Embedding + vector store + MongoDB chunks (sets status=indexing, progress=0)
	if err := s.AddChunks(docID, allChunks); err != nil {
		return fmt.Errorf("add chunks: %w", err)
	}

	// 5. Mark as ready with 100% progress
	return s.kb.UpdateDocStatus(ctx, docID, knowledge.StatusReady, len(allChunks), 100)
}

const chunkWindowSize = 4096

// splitBySentence reads text in fixed-size windows, splitting at sentence
// boundaries (。 or . followed by whitespace/newline). If 3 consecutive
// windows have no boundary, the accumulated text is yielded as-is.
func splitBySentence(text string, windowSize int) []string {
	var segments []string
	var buf []rune
	runes := []rune(text)
	noBoundaryCount := 0

	for i := 0; i < len(runes); {
		end := i + windowSize
		if end > len(runes) {
			end = len(runes)
		}
		buf = append(buf, runes[i:end]...)

		// Find the last sentence boundary in the buffer.
		boundaryIdx := lastSentenceBoundary(buf)
		if boundaryIdx >= 0 {
			// Split: everything up to and including the boundary.
			segments = append(segments, string(buf[:boundaryIdx+1]))
			buf = buf[boundaryIdx+1:]
			noBoundaryCount = 0
		} else {
			noBoundaryCount++
			if noBoundaryCount >= 3 {
				// Force yield after 3 windows without a boundary.
				segments = append(segments, string(buf))
				buf = nil
				noBoundaryCount = 0
			}
		}
		i = end
	}

	// Flush remaining text.
	if len(buf) > 0 {
		segments = append(segments, string(buf))
	}
	return segments
}

// lastSentenceBoundary returns the index of the last sentence boundary
// character in the buffer (。 or . when followed by space/newline/end), or -1.
func lastSentenceBoundary(buf []rune) int {
	for i := len(buf) - 1; i >= 0; i-- {
		if buf[i] == '。' {
			return i
		}
		if buf[i] == '.' {
			// English period: only count as sentence boundary when followed
			// by whitespace, newline, end-of-buffer, or another punctuation.
			if i+1 >= len(buf) || isSentenceEnd(rune(buf[i+1])) {
				return i
			}
		}
	}
	return -1
}

func isSentenceEnd(r rune) bool {
	return r == ' ' || r == '\n' || r == '\t' || r == '\r' ||
		r == '。' || r == '！' || r == '？' || r == '.' || r == '!' || r == '?'
}

type SearchResponse struct {
	Results []SearchResult `json:"results"`
}

type SearchResult struct {
	ChunkID string  `json:"chunk_id"`
	Text    string  `json:"text,omitempty"`
	Score   float64 `json:"score"`
}

// SetPublicFlag toggles the is_public flag on a doc, its chunks in MongoDB,
// and updates all vector payloads in Qdrant.
func (s *Service) SetPublicFlag(ctx context.Context, docID string, isPublic bool) error {
	if err := s.kb.SetPublicFlag(ctx, docID, isPublic); err != nil {
		return fmt.Errorf("set public flag on doc: %w", err)
	}
	// Update chunk visibility in MongoDB
	if err := s.kb.UpdateChunkVisibility(ctx, docID, isPublic); err != nil {
		log.Printf("[kb] update chunk visibility failed: %v", err)
	}
	// Update vector payloads in Qdrant if available
	if s.vector != nil {
		if err := s.vector.SetPayload(ctx, s.vecCol, docID, map[string]interface{}{"is_public": isPublic}); err != nil {
			log.Printf("[kb] update qdrant payload for doc=%s: %v", docID, err)
		}
	}
	return nil
}

// genShortID generates a short unique identifier.
func genShortID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())[:12]
}

func rrfFusion(list1, list2 []knowledge.SearchResult, topK int, k float64) []knowledge.SearchResult {
	scores := make(map[string]float64)
	results := make(map[string]knowledge.SearchResult)

	for i, r := range list1 {
		score := 1.0 / (k + float64(i+1))
		scores[r.ChunkID] += score
		results[r.ChunkID] = r
	}
	for i, r := range list2 {
		score := 1.0 / (k + float64(i+1))
		scores[r.ChunkID] += score
		if _, exists := results[r.ChunkID]; !exists {
			results[r.ChunkID] = r
		}
	}

	type scored struct {
		id    string
		score float64
	}
	var sorted []scored
	for id, score := range scores {
		sorted = append(sorted, scored{id, score})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})

	var fused []knowledge.SearchResult
	for i := 0; i < int(math.Min(float64(topK), float64(len(sorted)))); i++ {
		r := results[sorted[i].id]
		r.Score = sorted[i].score
		fused = append(fused, r)
	}
	return fused
}
