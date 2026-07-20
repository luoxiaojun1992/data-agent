package audit

//go:generate mockery --name AuditService --output ./mocks --outpkg mocks

// AuditService defines the audit service contract.
type AuditService interface {
	List(p ListParams) (*ListResult, error)
}
