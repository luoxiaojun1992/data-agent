package apireview

import (
	"context"
	"fmt"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/apireview"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const collAPIReviews = "api_reviews"

// Service handles API review CRUD and approval logic.
type Service struct {
	coll *mongo.Collection
}

// NewService creates an API review service.
func NewService(db *mongo.Database) *Service {
	return &Service{coll: db.Collection(collAPIReviews)}
}

// Create submits a new API conversion for review.
func (s *Service) Create(name, fileName, domain string, version string, endpoints, rateLimit int, submitter string) (*apireview.APIReview, error) {
	now := time.Now()
	r := &apireview.APIReview{
		ID:        "apirev_" + genShortID(),
		Name:      name,
		FileName:  fileName,
		Version:   version,
		Endpoints: endpoints,
		Domain:    domain,
		RateLimit: rateLimit,
		Submitter: submitter,
		Status:    apireview.StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := s.coll.InsertOne(context.Background(), r)
	if err != nil {
		return nil, fmt.Errorf("insert api review: %w", err)
	}
	return r, nil
}

// ListAll returns all API reviews, newest first.
func (s *Service) ListAll() ([]apireview.APIReview, error) {
	opts := options.Find().SetSort(bson.M{"created_at": -1}).SetLimit(100)
	cursor, err := s.coll.Find(context.Background(), bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())
	var reviews []apireview.APIReview
	if err := cursor.All(context.Background(), &reviews); err != nil {
		return nil, err
	}
	if reviews == nil {
		reviews = []apireview.APIReview{}
	}
	return reviews, nil
}

// Approve approves an API review. Returns error if reviewer is the submitter.
func (s *Service) Approve(id, reviewer string) error {
	var r apireview.APIReview
	err := s.coll.FindOne(context.Background(), bson.M{"_id": id}).Decode(&r)
	if err != nil {
		return fmt.Errorf("find api review: %w", err)
	}
	if r.Status != apireview.StatusPending {
		return fmt.Errorf("only pending reviews can be approved")
	}
	if r.Submitter == reviewer {
		return fmt.Errorf("不可审核自己提交的转换")
	}
	now := time.Now()
	_, err = s.coll.UpdateOne(context.Background(),
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"status": apireview.StatusApproved, "reviewer": reviewer, "reviewed_at": now, "updated_at": now}},
	)
	return err
}

// Reject rejects an API review with a reason.
func (s *Service) Reject(id, reviewer, reason string) error {
	if reason == "" {
		return fmt.Errorf("驳回原因不能为空")
	}
	var r apireview.APIReview
	err := s.coll.FindOne(context.Background(), bson.M{"_id": id}).Decode(&r)
	if err != nil {
		return fmt.Errorf("find api review: %w", err)
	}
	if r.Status != apireview.StatusPending {
		return fmt.Errorf("only pending reviews can be rejected")
	}
	now := time.Now()
	_, err = s.coll.UpdateOne(context.Background(),
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"status": apireview.StatusRejected, "reviewer": reviewer, "reject_reason": reason, "reviewed_at": now, "updated_at": now}},
	)
	return err
}

func genShortID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())[:12]
}
