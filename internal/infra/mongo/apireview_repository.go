package mongo

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/repository"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// APIReviewRepository implements repository.APIReviewRepository backed by MongoDB.
type APIReviewRepository struct {
	coll *mongo.Collection
}

func NewAPIReviewRepository(db *mongo.Database) *APIReviewRepository {
	return &APIReviewRepository{coll: db.Collection("api_reviews")}
}

func (r *APIReviewRepository) Create(ctx context.Context, review map[string]interface{}) error {
	review["status"] = "pending"
	_, err := r.coll.InsertOne(ctx, review)
	return err
}

func (r *APIReviewRepository) List(ctx context.Context, skip, limit int64) ([]map[string]interface{}, error) {
	opts := options.Find().SetSort(bson.M{"created_at": -1}).SetSkip(skip).SetLimit(limit)
	cursor, err := r.coll.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var results []map[string]interface{}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *APIReviewRepository) FindByID(ctx context.Context, id string) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&result)
	return result, err
}

func (r *APIReviewRepository) UpdateStatus(ctx context.Context, id string, update map[string]interface{}) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": update})
	return err
}

var _ repository.APIReviewRepository = (*APIReviewRepository)(nil)
