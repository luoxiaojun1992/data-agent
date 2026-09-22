package audit

import "github.com/luoxiaojun1992/data-agent/internal/domain/model"

//go:generate mockery --name AuditService --output ./mocks --outpkg mocks

// AuditService defines the audit service contract.
type AuditService interface {
	List(p ListParams) (*ListResult, error)
	Export(p ExportParams) ([]model.AuditLog, error)
}
