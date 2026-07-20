package mongo

import (
	"context"
	"time"

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

// Count returns the count of matching audit logs.
func (r *AuditRepository) Count(ctx context.Context, filter map[string]interface{}) (int64, error) {
	return r.coll.CountDocuments(ctx, toBson(filter))
}

// List returns audit logs matching the filter.
func (r *AuditRepository) List(ctx context.Context, filter map[string]interface{}, skip, limit int64) ([]map[string]interface{}, error) {
	opts := options.Find().SetSort(bson.M{"created_at": -1}).SetSkip(skip).SetLimit(limit)
	cursor, err := r.coll.Find(ctx, toBson(filter), opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []map[string]interface{}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	if results == nil {
		results = []map[string]interface{}{}
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

// normalizeLimit clamps limit to valid bounds.
func normalizeLimit(limit int64) int64 {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

// parseDateRange parses start/end date strings.
func parseDateRange(start, end string) (bson.M, error) {
	if start == "" && end == "" {
		return nil, nil
	}
	df := bson.M{}
	if start != "" {
		t, err := time.Parse("2006-01-02", start)
		if err != nil {
			return nil, err
		}
		df["$gte"] = t
	}
	if end != "" {
		t, err := time.Parse("2006-01-02", end)
		if err != nil {
			return nil, err
		}
		df["$lt"] = t.Add(24 * time.Hour)
	}
	return df, nil
}
