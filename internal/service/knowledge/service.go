package knowledge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
	task "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// ErrNotFound is returned when a knowledge doc does not exist OR does not belong
// to the caller (SPEC-084 §6.7 IDOR protection — existence is never leaked).
var ErrNotFound = errors.New("not found")

type EmbeddingFunc func(ctx context.Context, text string) ([]float32, error)

type Service struct {
	kb     repository.KBRepository
	vector repository.VectorRepository
	embed  EmbeddingFunc
	vecCol string
	// graph is the optional graph index (SPEC-070). nil = graph indexing off.
	graph    repository.GraphRepository
	graphTopN int
	// redactor is the optional PII redactor (SPEC-068). redactionEnabled reads
	// the `pii_redaction_enabled` switch; nil redactor = no redaction.
	redactor         security.Redactor
	redactionEnabled func() bool
	// queue is the optional async index queue (SPEC-086). nil = no enqueue
	// (CreateTextDoc still creates the doc synchronously).
	queue repository.QueueRepository
}

func NewService(kb repository.KBRepository) *Service {
	return &Service{kb: kb, vecCol: "kb_chunks", graphTopN: 5}
}

func (s *Service) WithVectorIndex(repo repository.VectorRepository, embed EmbeddingFunc) *Service {
	s.vector = repo
	s.embed = embed
	return s
}

// WithGraphIndex injects the graph repository (SPEC-070). Each chunk links to
// at most graphTopN similar chunks of the same creator.
func (s *Service) WithGraphIndex(graph repository.GraphRepository) *Service {
	s.graph = graph
	return s
}

// WithGraphTopN overrides the default 5 for the RELATED_TO fan-out.
func (s *Service) WithGraphTopN(topN int) *Service {
	if topN > 0 {
		s.graphTopN = topN
	}
	return s
}

// WithRedactor injects the PII redactor and the switch reader. enabled==nil
// means "always on" when a redactor is present.
func (s *Service) WithRedactor(r security.Redactor, enabled func() bool) *Service {
	s.redactor = r
	s.redactionEnabled = enabled
	return s
}

// WithQueue injects the async index queue (SPEC-086). nil = no enqueue; the
// doc is still created synchronously and indexing must be triggered elsewhere.
func (s *Service) WithQueue(q repository.QueueRepository) *Service {
	s.queue = q
	return s
}

// maybeRedact redacts pure text before it lands in the knowledge base.
// Semantics (SPEC-068 §4.6): switch off → skip (keep original); switch on +
// redactor error → fail-closed (return error); switch on + success → redacted.
func (s *Service) maybeRedact(ctx context.Context, text string) (string, error) {
	if s.redactor == nil {
		return text, nil
	}
	if s.redactionEnabled != nil && !s.redactionEnabled() {
		return text, nil // switch off → skip redaction
	}
	redacted, err := s.redactor.Redact(ctx, text)
	if err != nil {
		return "", fmt.Errorf("pii redact: %w", err) // fail-closed
	}
	return redacted, nil
}

// RedactText redacts a plain-text body (used by file upload before GridFS).
func (s *Service) RedactText(ctx context.Context, text string) (string, error) {
	return s.maybeRedact(ctx, text)
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

// CreateTextDoc creates a plain-text KB doc end-to-end (SPEC-086 §5.3):
// length check → title truncation → PII redaction → GridFS upload → CreateDoc
// → async index enqueue. Returns the created doc (status=uploaded) without
// waiting for indexing. userID is mandatory (caller enforces; empty would
// create an orphan doc).
func (s *Service) CreateTextDoc(ctx context.Context, userID, title, text string) (*knowledge.KnowledgeDoc, error) {
	if userID == "" {
		return nil, errors.New("kb_create_doc: userID is required")
	}
	if len(text) > knowledge.MaxKBTextBytes {
		return nil, fmt.Errorf("文本超过 %dMB 上限", knowledge.MaxKBTextBytes/(1024*1024))
	}
	// Title is a display label — truncate on overflow (SPEC-081 §5.3), never reject.
	title = truncateRunes(title, knowledge.MaxKBTitleRunes)
	if title == "" {
		return nil, errors.New("kb_create_doc: title is required")
	}

	redacted, err := s.RedactText(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("pii redact: %w", err)
	}

	fileName := title + ".txt"
	gridFSFileID, err := s.UploadFile(fileName, "text/plain", bytes.NewReader([]byte(redacted)))
	if err != nil {
		return nil, err
	}

	doc, err := s.CreateDoc(userID, title, fileName, knowledge.FileTypeTxt, int64(len(redacted)), gridFSFileID)
	if err != nil {
		return nil, err
	}

	// Async index (best-effort). nil queue → skip with a log (doc remains
	// uploaded, indexing must be triggered elsewhere).
	if s.queue != nil {
		if err := s.queue.EnqueueRaw(ctx, "kb_index", task.KBIndexPayload{
			DocID:        doc.ID,
			GridFSFileID: gridFSFileID,
		}); err != nil {
			log.Printf("[kb] CreateTextDoc failed to enqueue index job for doc=%s: %v", doc.ID, err)
		}
	}
	return doc, nil
}

// truncateRunes truncates s to at most n runes, keeping it a valid UTF-8 string.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func (s *Service) GetDoc(id, userID string, isSystemAdmin bool) (*knowledge.KnowledgeDoc, error) {
	doc, err := s.kb.GetDoc(context.Background(), id)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, nil
	}
	// SPEC-084 §6.7: owner 或 public 可读；system_admin 豁免。他人私有 doc
	// 返回 ErrNotFound（不泄露资源存在性）。
	if !isSystemAdmin && doc.UserID != userID && !doc.IsPublic {
		return nil, ErrNotFound
	}
	return doc, nil
}

func (s *Service) DeleteDoc(id, userID string, isSystemAdmin bool) error {
	// SPEC-070 五步级联（顺序：先删索引，再删明细，最后删主记录 doc）：
	// ① Qdrant（索引）→ ② ArcadeDB（索引）→ ③ chunks（明细）→
	// ④ GridFS 文件（明细）→ ⑤ doc（主记录 = 最终提交点）。
	// 索引/明细先删：中途失败时 doc 仍在，可重新索引自愈。
	// 五处均幂等（Qdrant filter 删 / ArcadeDB MATCH 删 / Mongo 永不 404 /
	// GridFS not-found 忽略），重试安全。
	doc, err := s.kb.GetDoc(context.Background(), id)
	if err != nil {
		// 幂等：doc 不存在（可能已删，重试场景）→ no-op 成功。其他错误
		// log 降级（无法拿到 GridFSFileID，后续删除无意义）。
		log.Printf("[kb] get doc for delete id=%s: %v", id, err)
		return nil
	}
	if doc == nil {
		// 无 doc → 幂等 no-op（重试场景）。
		return nil
	}
	// SPEC-084 §6.7: owner 或 public 可删；system_admin 豁免。他人私有 doc
	// 返回 ErrNotFound（不泄露资源存在性）。
	if !isSystemAdmin && doc.UserID != userID && !doc.IsPublic {
		return ErrNotFound
	}
	// ① Qdrant 向量（索引）
	if s.vector != nil {
		if err := s.vector.DeletePoints(context.Background(), s.vecCol, docIDFilter(id)); err != nil {
			log.Printf("[kb] delete qdrant points for doc=%s: %v", id, err)
		}
	}
	// ② ArcadeDB 图（索引）
	if s.graph != nil {
		if err := s.graph.DeleteByDocID(context.Background(), id); err != nil {
			log.Printf("[kb] delete graph chunks for doc=%s: %v", id, err)
		}
	}
	// ③ MongoDB chunks（明细）
	if _, err := s.kb.DeleteChunks(context.Background(), id); err != nil {
		log.Printf("[kb] delete chunks for doc=%s: %v", id, err)
	}
	// ④ GridFS 原始文件（明细）
	if err := s.kb.DeleteFile(context.Background(), doc.GridFSFileID); err != nil {
		log.Printf("[kb] delete gridfs file for doc=%s: %v", id, err)
	}
	// ⑤ MongoDB doc（主记录 = 最终提交点）
	if err := s.kb.DeleteDoc(context.Background(), id); err != nil {
		return err
	}
	return nil
}

// docIDFilter builds a Qdrant filter matching all points of one document.
func docIDFilter(docID string) map[string]interface{} {
	return map[string]interface{}{
		"must": []interface{}{
			map[string]interface{}{"key": "doc_id", "match": map[string]interface{}{"value": docID}},
		},
	}
}

// creatorFilter builds a Qdrant filter matching chunks of one creator
// (used by graph indexing: RELATED_TO edges only link same-creator chunks).
func creatorFilter(creatorID string) map[string]interface{} {
	return map[string]interface{}{
		"must": []interface{}{
			map[string]interface{}{"key": "creator_id", "match": map[string]interface{}{"value": creatorID}},
		},
	}
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
// q filters by title/file_name at the DB layer (SPEC-075); empty = no filter.
func (s *Service) ListDocsByVisibility(userID string, isSystemAdmin bool, q string, page, pageSize int) ([]*knowledge.KnowledgeDoc, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	skip := int64((page - 1) * pageSize)
	return s.kb.ListDocsByVisibility(context.Background(), userID, isSystemAdmin, q, skip, int64(pageSize))
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
	// SPEC-070: graph writes accumulate here and flush after Qdrant upsert
	// (先搜后写 — the chunk is not yet in Qdrant during the similarity
	// search, so it can never match itself).
	type graphWrite struct {
		chunk *knowledge.Chunk
		refs  []repository.RelatedRef
	}
	var graphWrites []graphWrite
	for idx, text := range texts {
		redacted, rErr := s.maybeRedact(context.Background(), text)
		if rErr != nil {
			return fmt.Errorf("pii redact chunk %d: %w", idx, rErr)
		}
		chunk := &knowledge.Chunk{
			ID:        "chunk_" + uuid.New().String(),
			DocID:     docID,
			CreatorID: doc.UserID,
			IsPublic:  doc.IsPublic,
			Content:   redacted,
			ChunkIdx:  idx,
			CharCount: len([]rune(redacted)),
		}
		chunks = append(chunks, chunk)
		if s.embed != nil && s.vector != nil {
			vec, err := s.embed(context.Background(), redacted)
			if err != nil {
				log.Printf("[kb] embed failed for chunk=%s: %v", chunk.ID, err)
				continue
			}
			if vec == nil {
				log.Printf("[kb] embed returned nil for chunk=%s (check embedding config)", chunk.ID)
				continue
			}
			// SPEC-070 ①: search topN similar chunks of the SAME creator
			// before upserting this chunk (it cannot match itself).
			if s.graph != nil {
				hits, sErr := s.vector.Search(context.Background(), s.vecCol, vec, s.graphTopN, creatorFilter(doc.UserID))
				if sErr == nil {
					refs := make([]repository.RelatedRef, 0, len(hits))
					for _, h := range hits {
						cid, _ := h.Metadata["chunk_id"].(string)
						if cid == "" || cid == chunk.ID {
							continue
						}
						refs = append(refs, repository.RelatedRef{ChunkID: cid, Score: float64(h.Score)})
					}
					graphWrites = append(graphWrites, graphWrite{chunk: chunk, refs: refs})
				} else {
					graphWrites = append(graphWrites, graphWrite{chunk: chunk})
				}
			}
			vectors = append(vectors, repository.VectorPoint{
				ID:     chunk.ID,
				Vector: vec,
				Metadata: map[string]interface{}{
					"chunk_id":   chunk.ID, // SPEC-070: 供图索引从 Search 命中反查原始 chunk ID
					"doc_id":     docID,
					"content":    redacted,
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
	// SPEC-070 ③: graph writes flush AFTER the Qdrant upsert. Failures degrade
	// with a log — the main indexing chain must not fail on graph errors.
	if s.graph != nil {
		for _, gw := range graphWrites {
			gc := repository.GraphChunk{
				ChunkID:   gw.chunk.ID,
				DocID:     gw.chunk.DocID,
				ChunkIdx:  gw.chunk.ChunkIdx,
				CreatorID: gw.chunk.CreatorID,
				IsPublic:  gw.chunk.IsPublic,
				CharCount: gw.chunk.CharCount,
			}
			if err := s.graph.UpsertChunk(context.Background(), gc); err != nil {
				log.Printf("[kb] graph upsert chunk=%s: %v", gw.chunk.ID, err)
				continue
			}
			if len(gw.refs) > 0 {
				if err := s.graph.LinkRelated(context.Background(), gw.chunk.ID, gw.refs); err != nil {
					log.Printf("[kb] graph link related chunk=%s: %v", gw.chunk.ID, err)
				}
			}
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

// DownloadFile returns the raw bytes of a GridFS-stored file.
func (s *Service) DownloadFile(ctx context.Context, fileID string) ([]byte, error) {
	return s.kb.DownloadFile(ctx, fileID)
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

	// 2-5. Index the downloaded text content.
	return s.indexContent(ctx, docID, string(data), llmChunkFn)
}

// IndexContent indexes pre-parsed text content directly (skipping the GridFS
// download). Used for images whose multimodal LLM parse already produced the
// text; the rest of the pipeline (sentence split → semantic chunk → embed →
// store) is identical to the TXT path.
func (s *Service) IndexContent(ctx context.Context, docID, text string, llmChunkFn func(text string) ([]string, error)) error {
	return s.indexContent(ctx, docID, text, llmChunkFn)
}

// indexContent runs the shared indexing pipeline over a text string.
func (s *Service) indexContent(ctx context.Context, docID, text string, llmChunkFn func(text string) ([]string, error)) error {
	// Split into sentence-boundary segments via fixed-window reading.
	//    Avoids loading giant files entirely into memory for LLM chunking.
	segments := splitBySentence(text, chunkWindowSize)

	// LLM semantic chunking per segment
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

	// Embedding + vector store + MongoDB chunks (sets status=indexing, progress=0)
	if err := s.AddChunks(docID, allChunks); err != nil {
		return fmt.Errorf("add chunks: %w", err)
	}

	// Mark as ready with 100% progress
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
// its vector payloads in Qdrant, and its chunk nodes in ArcadeDB (SPEC-091).
func (s *Service) SetPublicFlag(ctx context.Context, docID string, isPublic bool, userID string, isSystemAdmin bool) error {
	doc, err := s.kb.GetDoc(ctx, docID)
	if err != nil {
		return fmt.Errorf("get doc: %w", err)
	}
	if doc == nil {
		return ErrNotFound
	}
	// SPEC-084 §6.7: owner 或 public 可改 shared；system_admin 豁免。他人私有
	// doc 返回 ErrNotFound（不泄露资源存在性）。
	if !isSystemAdmin && doc.UserID != userID && !doc.IsPublic {
		return ErrNotFound
	}

	// SPEC-091 §4.3: 副作用（chunks / Qdrant / 图谱）先行，doc 的 is_public
	// 最后更新（提交点）。任一中间步骤失败即中断（return error）、不更新 doc
	// 字段（保持一致，可重试自愈）；失败不回滚已成功的副作用（容错不回滚）。
	// ① MongoDB chunks
	if err := s.kb.UpdateChunkVisibility(ctx, docID, isPublic); err != nil {
		return fmt.Errorf("set chunk visibility: %w", err)
	}
	// ② Qdrant vector payload
	if s.vector != nil {
		if err := s.vector.SetPayload(ctx, s.vecCol, docID, map[string]interface{}{"is_public": isPublic}); err != nil {
			return fmt.Errorf("set qdrant payload: %w", err)
		}
	}
	// ③ ArcadeDB graph nodes (SPEC-091)
	if s.graph != nil {
		if err := s.graph.SetDocPublic(ctx, docID, isPublic); err != nil {
			return fmt.Errorf("set graph is_public: %w", err)
		}
	}
	// ④ 提交点——最后更新 doc 的 is_public（source of truth）
	if err := s.kb.SetPublicFlag(ctx, docID, isPublic); err != nil {
		return fmt.Errorf("set public flag on doc: %w", err)
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
