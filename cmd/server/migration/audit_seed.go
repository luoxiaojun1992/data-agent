package migration

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SeedAudit idempotently migrates the audit_logs TTL index to a one-year
// retention (SPEC-102 D7). The legacy unnamed 90-day TTL index (created_at_1)
// is dropped first, then a named TTL index (ttl_created_at) is recreated with
// expireAfterSeconds = 365 days. No data is deleted synchronously — MongoDB's
// TTL monitor purges expired documents in the background.
func SeedAudit(ctx context.Context, db *mongo.Database) error {
	coll := db.Collection(model.CollAuditLogs)

	// Drop the legacy unnamed TTL index. DropOne returns (nil, nil) when the
	// index does not exist, so this is safe on fresh databases.
	_, _ = coll.Indexes().DropOne(ctx, "created_at_1")

	// TTL index {created_at:1}: 365 days auto-cleanup.
	_, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "created_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(31536000).SetName("ttl_created_at"),
	})
	return err
}
