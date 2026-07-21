package mongo

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/apireview"
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

func (r *APIReviewRepository) Create(ctx context.Context, review *apireview.APIReview) error {
	_, err := r.coll.InsertOne(ctx, apiReviewToDoc(review))
	return err
}

func (r *APIReviewRepository) List(ctx context.Context, skip, limit int64) ([]apireview.APIReview, error) {
	opts := options.Find().SetSort(bson.M{"created_at": -1}).SetSkip(skip).SetLimit(limit)
	cursor, err := r.coll.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	results := make([]apireview.APIReview, len(docs))
	for i, d := range docs {
		results[i] = *docToAPIReview(d)
	}
	return results, nil
}

func (r *APIReviewRepository) FindByID(ctx context.Context, id string) (*apireview.APIReview, error) {
	var d bson.M
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&d)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return docToAPIReview(d), nil
}

func (r *APIReviewRepository) UpdateStatus(ctx context.Context, id string, update map[string]interface{}) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": update})
	return err
}

var _ repository.APIReviewRepository = (*APIReviewRepository)(nil)
