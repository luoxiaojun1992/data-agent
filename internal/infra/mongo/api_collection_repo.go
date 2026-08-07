package mongo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
)

// APICollectionRepo wraps the api_collections MongoDB collection.
type APICollectionRepo struct {
	coll *mongo.Collection
}

// NewAPICollectionRepo creates a new APICollectionRepo.
func NewAPICollectionRepo(db *mongo.Database) *APICollectionRepo {
	return &APICollectionRepo{coll: db.Collection("api_collections")}
}

// EnsureIndexes creates indexes on api_collections.
func (r *APICollectionRepo) EnsureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}}},
		{Keys: bson.D{{Key: "status", Value: 1}}},
	}
	_, err := r.coll.Indexes().CreateMany(ctx, indexes)
	return err
}

// Create inserts a new API collection.
func (r *APICollectionRepo) Create(ctx context.Context, c *model.APICollection) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	now := time.Now()
	c.CreatedAt = now
	c.UpdatedAt = now
	if c.Status == "" {
		c.Status = model.APICollectionPending
	}
	_, err := r.coll.InsertOne(ctx, c)
	return err
}

// GetByID returns a single collection by ID.
func (r *APICollectionRepo) GetByID(ctx context.Context, id string) (*model.APICollection, error) {
	var c model.APICollection
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&c)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListParams holds pagination and filter parameters for listing collections.
type ListParams struct {
	UserID   string
	Page     int
	PageSize int
}

// ListResult holds paginated results.
type ListResult struct {
	Items    []*model.APICollection `json:"items"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
}

// List returns paginated collections. If UserID is set, filters by uploader.
func (r *APICollectionRepo) List(ctx context.Context, p ListParams) (*ListResult, error) {
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.PageSize <= 0 || p.PageSize > 50 {
		p.PageSize = 20
	}
	filter := bson.M{}
	if p.UserID != "" {
		filter["user_id"] = p.UserID
	}
	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, err
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(int64((p.Page - 1) * p.PageSize)).
		SetLimit(int64(p.PageSize))
	cur, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var items []*model.APICollection
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	return &ListResult{Items: items, Total: total, Page: p.Page, PageSize: p.PageSize}, nil
}

// UpdateFields sets name and description.
func (r *APICollectionRepo) UpdateFields(ctx context.Context, id, userID, name, desc string) error {
	update := bson.M{"$set": bson.M{"name": name, "description": desc, "updated_at": time.Now()}}
	filter := bson.M{"_id": id}
	if userID != "" {
		filter["user_id"] = userID
	}
	res, err := r.coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

// UpdateStatus sets the approval status.
func (r *APICollectionRepo) UpdateStatus(ctx context.Context, id string, status model.APICollectionStatus) error {
	update := bson.M{"$set": bson.M{"status": status, "updated_at": time.Now()}}
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, update)
	return err
}

// Delete removes a collection. If userID is set, only deletes own.
func (r *APICollectionRepo) Delete(ctx context.Context, id, userID string) error {
	filter := bson.M{"_id": id}
	if userID != "" {
		filter["user_id"] = userID
	}
	res, err := r.coll.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

// SearchByDescription performs fuzzy text search on the description field.
// Only approved collections are returned.
func (r *APICollectionRepo) SearchByDescription(ctx context.Context, query string, limit int) ([]*model.APICollection, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	filter := bson.M{
		"status": model.APICollectionApproved,
		"description": bson.M{"$regex": query, "$options": "i"},
	}
	opts := options.Find().SetLimit(int64(limit)).SetSort(bson.D{{Key: "created_at", Value: -1}})
	cur, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var items []*model.APICollection
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}
