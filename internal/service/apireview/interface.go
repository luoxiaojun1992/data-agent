package apireview

//go:generate mockery --name APIReviewService --output ./mocks --outpkg mocks

import "github.com/luoxiaojun1992/data-agent/internal/domain/apireview"

// APIReviewService defines the API review service contract.
type APIReviewService interface {
	Create(name, fileName, domain string, version string, endpoints, rateLimit int, submitter string) (*apireview.APIReview, error)
	ListAll() ([]apireview.APIReview, error)
	Approve(id, reviewer string) error
	Reject(id, reviewer, reason string) error
}

// Ensure *Service implements APIReviewService.
var _ APIReviewService = (*Service)(nil)
