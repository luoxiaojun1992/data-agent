package mongo

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AuditRepository implements repository.AuditRepository backed by MongoDB.
type AuditRepository struct {
	coll *mongo.Collection
}

// NewAuditRepository creates a new AuditRepository.
func NewAuditRepository(db *mongo.Database) *AuditRepository {
	return &AuditRepository{coll: db.Collection(model.CollAuditLogs)}
}

// Create inserts a new audit log entry. The ID is generated here so callers
// (middleware/service) never touch mongo-driver.
func (r *AuditRepository) Create(ctx context.Context, log *model.AuditLog) error {
	log.ID = NewDomainID()
	_, err := r.coll.InsertOne(ctx, auditLogToDoc(log))
	return err
}

// Count returns the count of matching audit logs.
func (r *AuditRepository) Count(ctx context.Context, filter map[string]interface{}) (int64, error) {
	return r.coll.CountDocuments(ctx, toBson(filter))
}

// List returns audit logs matching the filter.
func (r *AuditRepository) List(ctx context.Context, filter map[string]interface{}, skip, limit int64) ([]model.AuditLog, error) {
	opts := options.Find().SetSort(bson.M{"created_at": -1}).SetSkip(skip).SetLimit(limit)
	cursor, err := r.coll.Find(ctx, toBson(filter), opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	results := make([]model.AuditLog, len(docs))
	for i, d := range docs {
		results[i] = *docToAuditLog(d)
	}
	return results, nil
}

func toBson(m map[string]interface{}) bson.M {
	if m == nil {
		return bson.M{}
	}
	result := bson.M{}
	for k, v := range m {
		result[k] = v
	}
	return result
}
