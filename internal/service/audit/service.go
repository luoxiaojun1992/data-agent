package audit

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	"go.mongodb.org/mongo-driver/bson"
)

// Service handles audit log queries.
type Service struct {
	repo repository.AuditRepository
}

// NewService creates an audit log service.
func NewService(repo repository.AuditRepository) *Service {
	return &Service{repo: repo}
}

// ListParams are the filter parameters for listing audit logs.
type ListParams struct {
	Action string
	UserID string
	Start  string
	End    string
	Skip   int64
	Limit  int64
}

// ListResult contains the audit log list and total count.
type ListResult struct {
	Logs  []model.AuditLog `json:"logs"`
	Total int64            `json:"total"`
}

// List returns audit logs matching the filter params.
func (s *Service) List(p ListParams) (*ListResult, error) {
	p.Limit = normalizeAuditLimit(p.Limit)
	filterMap := auditFilterToMap(p)

	total, err := s.repo.Count(context.Background(), filterMap)
	if err != nil {
		return nil, err
	}

	rawLogs, err := s.repo.List(context.Background(), filterMap, p.Skip, p.Limit)
	if err != nil {
		return nil, err
	}

	var logs []model.AuditLog
	for _, raw := range rawLogs {
		var log model.AuditLog
		b, _ := bson.Marshal(raw)
		bson.Unmarshal(b, &log)
		logs = append(logs, log)
	}
	if logs == nil {
		logs = []model.AuditLog{}
	}

	return &ListResult{Logs: logs, Total: total}, nil
}

func auditFilterToMap(p ListParams) map[string]interface{} {
	m := map[string]interface{}{}
	if p.Action != "" {
		m["action"] = p.Action
	}
	if p.UserID != "" {
		m["user_id"] = p.UserID
	}
	if p.Start != "" {
		m["start"] = p.Start
	}
	if p.End != "" {
		m["end"] = p.End
	}
	return m
}

func normalizeAuditLimit(limit int64) int64 {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

// ExportParams defines the export request.
type ExportParams struct {
	Action string `json:"action"`
	UserID string `json:"user_id"`
	Start  string `json:"start"`
	End    string `json:"end"`
	Limit  int64  `json:"limit"`
	Format string `json:"format"`
}
