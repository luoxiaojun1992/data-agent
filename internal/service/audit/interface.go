package audit

import "context"

//go:generate mockery --name AuditService --output ./mocks --outpkg mocks

// AuditService defines the audit service contract.
type AuditService interface {
	List(ctx context.Context, p ListParams) (*ListResult, error)
}
