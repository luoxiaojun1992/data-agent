package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Client wraps the MongoDB driver client with connection pool management.
type Client struct {
	client *mongo.Client
	db     *mongo.Database
}

// NewClient creates a new MongoDB client and verifies connectivity.
func NewClient(ctx context.Context, uri, database string) (*Client, error) {
	clientOpts := options.Client().
		ApplyURI(uri).
		SetMaxPoolSize(100).
		SetMinPoolSize(5).
		SetMaxConnIdleTime(5 * time.Minute)

	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", err)
	}

	// Verify connectivity
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("mongo ping: %w", err)
	}

	db := client.Database(database)
	return &Client{client: client, db: db}, nil
}

// DB returns the underlying MongoDB database instance.
func (c *Client) DB() *mongo.Database {
	return c.db
}

// Collection returns a MongoDB collection by name.
func (c *Client) Collection(name string) *mongo.Collection {
	return c.db.Collection(name)
}

// Disconnect closes the MongoDB connection.
func (c *Client) Disconnect(ctx context.Context) error {
	return c.client.Disconnect(ctx)
}

// UpsertOne performs an idempotent upsert based on filter.
// Returns the upserted/updated document ID.
func UpsertOne(ctx context.Context, coll *mongo.Collection, filter bson.M, doc interface{}) error {
	opts := options.Update().SetUpsert(true)
	update := bson.M{"$set": doc}
	_, err := coll.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("upsert one: %w", err)
	}
	return nil
}

// DeleteOne performs an idempotent delete — returns nil even if the document doesn't exist.
func DeleteOne(ctx context.Context, coll *mongo.Collection, filter bson.M) error {
	_, err := coll.DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("delete one: %w", err)
	}
	// Idempotent: don't return error if no document matched
	return nil
}

// EnsureIndexes creates required indexes for all collections.
func EnsureIndexes(ctx context.Context, db *mongo.Database) error {
	indexes := map[string][]mongo.IndexModel{
		"users": {
			{Keys: bson.D{{Key: "username", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "role", Value: 1}}},
		},
		"roles": {
			{Keys: bson.D{{Key: "name", Value: 1}}, Options: options.Index().SetUnique(true)},
		},
		"audit_logs": {
			{Keys: bson.D{{Key: "user_id", Value: 1}}},
			{Keys: bson.D{{Key: "created_at", Value: -1}}},
			{Keys: bson.D{{Key: "created_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(90 * 86400)}, // TTL 90 days
		},
		"notifications": {
			{Keys: bson.D{{Key: "created_at", Value: -1}}},
		},
	}

	for collName, idxs := range indexes {
		if _, err := db.Collection(collName).Indexes().CreateMany(ctx, idxs); err != nil {
			return fmt.Errorf("ensure indexes for %s: %w", collName, err)
		}
	}

	return nil
}
