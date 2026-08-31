package migration

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SeedStats idempotently initializes the stats_hourly collection's indexes
// (SPEC-072). No business data is pre-seeded — counts start at zero and are
// accumulated by埋点. No historical data is migrated/backfilled.
func SeedStats(ctx context.Context, db *mongo.Database) error {
	coll := db.Collection("stats_hourly")
	// Unique index {metric, hour}: used to locate the same-hour document for
	// the upsert $inc (filter by metric+hour, not _id).
	if _, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "metric", Value: 1}, {Key: "hour", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_metric_hour"),
	}); err != nil {
		return err
	}
	// TTL index {hour}: 365 days auto-cleanup.
	if _, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "hour", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(31536000).SetName("ttl_hour"),
	}); err != nil {
		return err
	}
	return nil
}
