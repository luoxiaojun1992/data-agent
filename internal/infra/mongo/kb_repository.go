package mongo

import (
	"bytes"
	"context"
	"errors"
	"io"
	"regexp"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/gridfs"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type KBRepository struct {
	db *mongo.Database
}

func NewKBRepository(db *mongo.Database) *KBRepository {
	return &KBRepository{db: db}
}

func (r *KBRepository) CreateDoc(ctx context.Context, doc *knowledge.KnowledgeDoc) error {
	_, err := r.db.Collection("knowledge_docs").InsertOne(ctx, knowledgeDocToDoc(doc))
	return err
}

func (r *KBRepository) GetDoc(ctx context.Context, id string) (*knowledge.KnowledgeDoc, error) {
	var d bson.M
	err := r.db.Collection("knowledge_docs").FindOne(ctx, bson.M{"_id": id}).Decode(&d)
	if err != nil {
		return nil, err
	}
	return docToKnowledgeDoc(d), nil
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
	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, 0, err
	}
	out := make([]*knowledge.KnowledgeDoc, len(docs))
	for i, d := range docs {
		out[i] = docToKnowledgeDoc(d)
	}
	return out, total, nil
}

func (r *KBRepository) ListAllDocs(ctx context.Context, skip, limit int64) ([]*knowledge.KnowledgeDoc, int64, error) {
	filter := bson.M{}
	total, _ := r.db.Collection("knowledge_docs").CountDocuments(ctx, filter)
	opts := options.Find().SetSort(bson.M{"created_at": -1}).SetSkip(skip).SetLimit(limit)
	cursor, err := r.db.Collection("knowledge_docs").Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, 0, err
	}
	out := make([]*knowledge.KnowledgeDoc, len(docs))
	for i, d := range docs {
		out[i] = docToKnowledgeDoc(d)
	}
	return out, total, nil
}

func (r *KBRepository) UpdateDocStatus(ctx context.Context, id string, status knowledge.DocStatus, chunkCount, progressPercent int) error {
	_, err := r.db.Collection("knowledge_docs").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{
		"status":           status,
		"chunk_count":      chunkCount,
		"progress_percent": progressPercent,
		"updated_at":       time.Now(),
	}})
	return err
}

func (r *KBRepository) AddChunks(ctx context.Context, chunks []*knowledge.Chunk) error {
	docs := make([]interface{}, len(chunks))
	for i, c := range chunks {
		docs[i] = chunkToDoc(c)
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

func (r *KBRepository) SearchChunks(ctx context.Context, query string, userID string, isSystemAdmin bool, topK int) ([]*knowledge.SearchResult, error) {
	filter := bson.M{"$text": bson.M{"$search": query}}
	if !isSystemAdmin {
		// Regular user: only visible chunks (own or public)
		filter["$or"] = []bson.M{
			{"creator_id": userID},
			{"is_public": true},
		}
	}
	cursor, err := r.db.Collection("kb_chunks").Find(ctx, filter, options.Find().SetLimit(int64(topK)))
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

// UploadFile stores file content in MongoDB GridFS under the given file ID.
func (r *KBRepository) UploadFile(ctx context.Context, fileID string, reader io.Reader) error {
	bucket, err := gridfs.NewBucket(r.db)
	if err != nil {
		return err
	}
	stream, err := bucket.OpenUploadStream(fileID)
	if err != nil {
		return err
	}
	defer stream.Close()
	_, err = io.Copy(stream, reader)
	return err
}

// DownloadFile retrieves file content from MongoDB GridFS by file ID.
func (r *KBRepository) DownloadFile(ctx context.Context, fileID string) ([]byte, error) {
	bucket, err := gridfs.NewBucket(r.db)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	_, err = bucket.DownloadToStreamByName(fileID, &buf)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DeleteFile removes a GridFS file. Idempotent: an empty fileID is a no-op
// and file-not-found is ignored (SPEC-070 cascade delete).
func (r *KBRepository) DeleteFile(ctx context.Context, fileID string) error {
	if fileID == "" {
		return nil
	}
	bucket, err := gridfs.NewBucket(r.db)
	if err != nil {
		return err
	}
	if err := bucket.Delete(fileID); err != nil && !errors.Is(err, gridfs.ErrFileNotFound) {
		return err
	}
	return nil
}

// SetPublicFlag toggles the is_public flag on a knowledge document.
func (r *KBRepository) SetPublicFlag(ctx context.Context, id string, isPublic bool) error {
	_, err := r.db.Collection("knowledge_docs").UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"is_public": isPublic, "updated_at": time.Now()}})
	return err
}

// UpdateChunkVisibility updates the is_public flag on all chunks of a document.
func (r *KBRepository) UpdateChunkVisibility(ctx context.Context, docID string, isPublic bool) error {
	_, err := r.db.Collection("kb_chunks").UpdateMany(ctx,
		bson.M{"doc_id": docID},
		bson.M{"$set": bson.M{"is_public": isPublic}})
	return err
}

// ListDocsByVisibility returns docs visible to the current user.
// System admin: all docs. Regular user: own docs + public docs.
// q filters by title/file_name (case-insensitive $regex, quote-meta escaped).
func (r *KBRepository) ListDocsByVisibility(ctx context.Context, userID string, isSystemAdmin bool, q string, skip, limit int64) ([]*knowledge.KnowledgeDoc, int64, error) {
	var visibility bson.M
	if isSystemAdmin {
		visibility = bson.M{}
	} else {
		visibility = bson.M{
			"$or": []bson.M{
				{"user_id": userID},
				{"is_public": true},
			},
		}
	}
	filter := visibility
	if q != "" {
		qre := bson.M{"$regex": regexp.QuoteMeta(q), "$options": "i"}
		qFilter := bson.M{"$or": []bson.M{
			{"title": qre},
			{"file_name": qre},
		}}
		if len(visibility) == 0 {
			filter = qFilter
		} else {
			filter = bson.M{"$and": []bson.M{visibility, qFilter}}
		}
	}
	total, _ := r.db.Collection("knowledge_docs").CountDocuments(ctx, filter)
	opts := options.Find().SetSort(bson.M{"created_at": -1}).SetSkip(skip).SetLimit(limit)
	cursor, err := r.db.Collection("knowledge_docs").Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, 0, err
	}
	out := make([]*knowledge.KnowledgeDoc, len(docs))
	for i, d := range docs {
		out[i] = docToKnowledgeDoc(d)
	}
	return out, total, nil
}

var _ repository.KBRepository = (*KBRepository)(nil)
