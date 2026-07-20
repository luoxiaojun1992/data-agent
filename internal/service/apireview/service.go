package apireview

import (
	"context"
	"fmt"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/apireview"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	"go.mongodb.org/mongo-driver/bson"
)

type Service struct {
	repo repository.APIReviewRepository
}

func NewService(repo repository.APIReviewRepository) *Service {
	return &Service{repo: repo}
}

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
	doc := map[string]interface{}{}
	b, _ := bson.Marshal(r)
	bson.Unmarshal(b, &doc)
	if err := s.repo.Create(context.Background(), doc); err != nil {
		return nil, fmt.Errorf("insert api review: %w", err)
	}
	return r, nil
}

func (s *Service) ListAll() ([]apireview.APIReview, error) {
	rawList, err := s.repo.List(context.Background(), 0, 100)
	if err != nil {
		return nil, err
	}
	var reviews []apireview.APIReview
	for _, raw := range rawList {
		var r apireview.APIReview
		b, _ := bson.Marshal(raw)
		bson.Unmarshal(b, &r)
		reviews = append(reviews, r)
	}
	return reviews, nil
}

func (s *Service) Approve(id string, reviewer ...string) error {
	_ = reviewer // backward compat — reviewers param kept for existing callers
	_, err := s.repo.FindByID(context.Background(), id)
	if err != nil {
		return fmt.Errorf("review not found: %w", err)
	}
	return s.repo.Approve(context.Background(), id)
}

func (s *Service) Reject(id string, reason string, reviewer ...string) error {
	_ = reviewer // backward compat
	_, err := s.repo.FindByID(context.Background(), id)
	if err != nil {
		return fmt.Errorf("review not found: %w", err)
	}
	return s.repo.Reject(context.Background(), id, reason)
}

func genShortID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())[:8]
}
