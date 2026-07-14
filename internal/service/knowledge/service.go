package knowledge

import (
	"context"
	"fmt"
	"io"
	"math"
	"sort"

	"github.com/google/uuid"
	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/gridfs"
)

// Service handles knowledge base operations.
type Service struct {
	db *mongo.Database
}

// NewService creates a knowledge base service.
func NewService(db *mongo.Database) *Service {
	return &Service{db: db}
}

// CreateDoc creates a new knowledge document record.
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
	_, err := s.db.Collection("knowledge_docs").InsertOne(context.Background(), doc)
	if err != nil {
		return nil, fmt.Errorf("insert knowledge doc: %w", err)
	}
	return doc, nil
}

// GetDoc retrieves a knowledge document by ID.
func (s *Service) GetDoc(docID string) (*knowledge.KnowledgeDoc, error) {
	var doc knowledge.KnowledgeDoc
	err := s.db.Collection("knowledge_docs").FindOne(context.Background(), bson.M{"_id": docID}).Decode(&doc)
	if err != nil {
		return nil, fmt.Errorf("find knowledge doc: %w", err)
	}
	return &doc, nil
}

// DeleteDoc removes a document and all its chunks (cascade).
func (s *Service) DeleteDoc(docID string) error {
	// Delete chunks
	_, err := s.db.Collection("kb_chunks").DeleteMany(context.Background(), bson.M{"doc_id": docID})
	if err != nil {
		return fmt.Errorf("delete chunks: %w", err)
	}
	// Delete document
	_, err = s.db.Collection("knowledge_docs").DeleteOne(context.Background(), bson.M{"_id": docID})
	if err != nil {
		return fmt.Errorf("delete doc: %w", err)
	}
	return nil
}

// ListDocs returns all documents for a user.
func (s *Service) ListDocs(userID string) ([]knowledge.KnowledgeDoc, error) {
	cursor, err := s.db.Collection("knowledge_docs").Find(context.Background(), bson.M{"user_id": userID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	var docs []knowledge.KnowledgeDoc
	if err := cursor.All(context.Background(), &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

// ListAllDocs returns all documents globally (admin view).
func (s *Service) ListAllDocs() ([]knowledge.KnowledgeDoc, error) {
	cursor, err := s.db.Collection("knowledge_docs").Find(context.Background(), bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	var docs []knowledge.KnowledgeDoc
	if err := cursor.All(context.Background(), &docs); err != nil {
		return nil, err
	}
	if docs == nil {
		docs = []knowledge.KnowledgeDoc{}
	}
	return docs, nil
}

// AddChunks inserts semantic chunks for a document.
func (s *Service) AddChunks(docID string, chunks []string) error {
	for i, content := range chunks {
		chunk := &knowledge.Chunk{
			ID:       fmt.Sprintf("chunk_%s_%d", docID, i),
			DocID:    docID,
			Content:  content,
			ChunkIdx: i,
			CharCount: len([]rune(content)),
		}
		_, err := s.db.Collection("kb_chunks").InsertOne(context.Background(), chunk)
		if err != nil {
			return fmt.Errorf("insert chunk: %w", err)
		}
	}
	// Update doc chunk count
	_, _ = s.db.Collection("knowledge_docs").UpdateOne(context.Background(),
		bson.M{"_id": docID},
		bson.M{"$set": bson.M{"chunk_count": len(chunks), "status": knowledge.StatusReady}},
	)
	return nil
}

// Search performs hybrid search across Milvus (semantic) and MongoDB (full-text).
// Uses Reciprocal Rank Fusion (RRF) to merge results.
func (s *Service) Search(userID, query string, topK int, role string) ([]knowledge.SearchResult, error) {
	// Full-text search (MongoDB)
	textResults := s.fullTextSearch(query, topK)

	// Milvus semantic search (placeholder — returns empty for MVP)
	semanticResults := s.semanticSearch(query, topK)

	// RRF fusion
	results := rrfFusion(textResults, semanticResults, topK, 60)

	// Permission filtering by role
	results = s.filterByRole(results, role)

	return results, nil
}

func (s *Service) fullTextSearch(query string, topK int) []knowledge.SearchResult {
	filter := bson.M{
		"$text": bson.M{"$search": query},
	}
	opts := map[string]interface{}{
		"score": bson.M{"$meta": "textScore"},
	}
	cursor, err := s.db.Collection("kb_chunks").Find(context.Background(), filter)
	if err != nil {
		return nil
	}
	defer cursor.Close(context.Background())

	var results []knowledge.SearchResult
	for cursor.Next(context.Background()) {
		var chunk knowledge.Chunk
		if err := cursor.Decode(&chunk); err != nil {
			continue
		}
		// Get doc title
		var doc knowledge.KnowledgeDoc
		_ = s.db.Collection("knowledge_docs").FindOne(context.Background(), bson.M{"_id": chunk.DocID}).Decode(&doc)

		_ = opts // textScore via meta
		results = append(results, knowledge.SearchResult{
			ChunkID:  chunk.ID,
			DocID:    chunk.DocID,
			DocTitle: doc.Title,
			Content:  chunk.Content,
			Score:    1.0,
			Source:   "fulltext",
		})
		if len(results) >= topK {
			break
		}
	}
	return results
}

func (s *Service) semanticSearch(query string, topK int) []knowledge.SearchResult {
	// Placeholder — Milvus integration in SPEC-009
	_ = query
	_ = topK
	return nil
}

// rrfFusion merges two ranked lists using Reciprocal Rank Fusion.
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

	// Sort by fused score
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

func (s *Service) filterByRole(results []knowledge.SearchResult, role string) []knowledge.SearchResult {
	// All roles can see their own docs; system_admin sees all
	if role == "system_admin" {
		return results
	}
	// Placeholder: filter by user_id in production
	return results
}

// UploadFile stores file content in MongoDB GridFS and returns the file ID.
func (s *Service) UploadFile(filename, contentType string, reader io.Reader) (string, error) {
	bucket, err := gridfs.NewBucket(s.db)
	if err != nil {
		return "", fmt.Errorf("gridfs bucket: %w", err)
	}
	fileID := "gridfs_" + genShortID()
	uploadStream, err := bucket.OpenUploadStream(fileID)
	if err != nil {
		return "", fmt.Errorf("gridfs open upload: %w", err)
	}
	defer uploadStream.Close()
	if _, err := io.Copy(uploadStream, reader); err != nil {
		return "", fmt.Errorf("gridfs write: %w", err)
	}
	return fileID, nil
}

func genShortID() string { return uuid.New().String()[:12] }
