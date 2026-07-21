package apireview

import (
	"context"
	"fmt"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/apireview"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
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
	if err := s.repo.Create(context.Background(), r); err != nil {
		return nil, fmt.Errorf("insert api review: %w", err)
	}
	return r, nil
}

func (s *Service) ListAll() ([]apireview.APIReview, error) {
	reviews, err := s.repo.List(context.Background(), 0, 100)
	if err != nil {
		return nil, err
	}
	if reviews == nil {
		reviews = []apireview.APIReview{}
	}
	return reviews, nil
}

func (s *Service) Approve(id, reviewer string) error {
	r, err := s.repo.FindByID(context.Background(), id)
	if err != nil {
		return fmt.Errorf("find api review: %w", err)
	}
	if r == nil {
		return fmt.Errorf("api review not found")
	}
	if r.Status != apireview.StatusPending {
		return fmt.Errorf("only pending reviews can be approved")
	}
	if r.Submitter == reviewer {
		return fmt.Errorf("不可审核自己提交的转换")
	}
	now := time.Now()
	return s.repo.UpdateStatus(context.Background(), id, map[string]interface{}{
		"status":      apireview.StatusApproved,
		"reviewer":    reviewer,
		"reviewed_at": now,
		"updated_at":  now,
	})
}

func (s *Service) Reject(id, reviewer, reason string) error {
	if reason == "" {
		return fmt.Errorf("驳回原因不能为空")
	}
	r, err := s.repo.FindByID(context.Background(), id)
	if err != nil {
		return fmt.Errorf("find api review: %w", err)
	}
	if r == nil {
		return fmt.Errorf("api review not found")
	}
	if r.Status != apireview.StatusPending {
		return fmt.Errorf("only pending reviews can be rejected")
	}
	now := time.Now()
	return s.repo.UpdateStatus(context.Background(), id, map[string]interface{}{
		"status":        apireview.StatusRejected,
		"reviewer":      reviewer,
		"reject_reason": reason,
		"reviewed_at":   now,
		"updated_at":    now,
	})
}

func genShortID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())[:8]
}
