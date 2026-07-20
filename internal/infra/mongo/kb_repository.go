package mongo

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type KBRepository struct {
	db *mongo.Database
}

func NewKBRepository(db *mongo.Database) *KBRepository {
	return &KBRepository{db: db}
}

func (r *KBRepository) CreateDoc(ctx context.Context, doc *knowledge.KnowledgeDoc) error {
	_, err := r.db.Collection("knowledge_docs").InsertOne(ctx, doc)
	return err
}

func (r *KBRepository) GetDoc(ctx context.Context, id string) (*knowledge.KnowledgeDoc, error) {
	var doc knowledge.KnowledgeDoc
	err := r.db.Collection("knowledge_docs").FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	return &doc, err
}

func (r *KBRepository) DeleteDoc(ctx context.Context, id string) error {
	_, err := r.db.Collection("knowledge_docs").DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *KBRepository) ListDocs(ctx context.Context, userID string, skip, limit int64) ([]*knowledge.KnowledgeDoc, int64, error) {
	filter := bson.M{"user_id": userID}
	total, _ := r.db.Collection("knowledge_docs").CountDocuments(ctx, filter)
	opts := options.Find().SetSort(bson.M{"created_at": -1}).SetSkip(skip).SetLimit(limit)
	cursor, err := r.db.Collection("knowledge_docs").Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var docs []*knowledge.KnowledgeDoc
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, 0, err
	}
	return docs, total, nil
}

func (r *KBRepository) ListAllDocs(ctx context.Context) ([]*knowledge.KnowledgeDoc, error) {
	cursor, err := r.db.Collection("knowledge_docs").Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var docs []*knowledge.KnowledgeDoc
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

func (r *KBRepository) UpdateDocStatus(ctx context.Context, id string, status knowledge.DocStatus, chunkCount int) error {
	_, err := r.db.Collection("knowledge_docs").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{
		"status":      status,
		"chunk_count": chunkCount,
	}})
	return err
}

func (r *KBRepository) AddChunks(ctx context.Context, chunks []*knowledge.Chunk) error {
	docs := make([]interface{}, len(chunks))
	for i, c := range chunks {
		docs[i] = c
	}
	_, err := r.db.Collection("kb_chunks").InsertMany(ctx, docs)
	return err
}

func (r *KBRepository) DeleteChunks(ctx context.Context, docID string) (int64, error) {
	res, err := r.db.Collection("kb_chunks").DeleteMany(ctx, bson.M{"doc_id": docID})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

func (r *KBRepository) SearchChunks(ctx context.Context, query string, topK int) ([]*knowledge.SearchResult, error) {
	cursor, err := r.db.Collection("kb_chunks").Find(ctx, bson.M{"$text": bson.M{"$search": query}}, options.Find().SetLimit(int64(topK)))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var results []*knowledge.SearchResult
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

var _ repository.KBRepository = (*KBRepository)(nil)
