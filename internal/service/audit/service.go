package audit

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// Service handles audit log queries.
type Service struct {
	repo     repository.AuditRepository
	userRepo repository.UserRepository
}

// NewService creates an audit log service.
func NewService(repo repository.AuditRepository, userRepo repository.UserRepository) *Service {
	return &Service{repo: repo, userRepo: userRepo}
}

// ListParams are the filter parameters for listing audit logs.
type ListParams struct {
	Action string
	UserID string // user search keyword — matches the user email, not the raw ID
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
//
// The user filter (`UserID`) is treated as an email keyword: it first looks
// up the top-10 matching users by email, then filters audit logs by their
// IDs via $in (all other filters and pagination are unchanged). On the way
// out, each log's raw user ID is replaced with the user's email.
func (s *Service) List(p ListParams) (*ListResult, error) {
	p.Limit = normalizeAuditLimit(p.Limit)
	ctx := context.Background()

	// Resolve the email keyword to a set of user IDs (top 10 matches).
	var userIDs []string
	if p.UserID != "" {
		if s.userRepo == nil {
			return nil, fmt.Errorf("user repository not available")
		}
		users, err := s.userRepo.SearchByEmail(ctx, p.UserID, 10)
		if err != nil {
			return nil, fmt.Errorf("search users by email: %w", err)
		}
		if len(users) == 0 {
			return &ListResult{Logs: []model.AuditLog{}, Total: 0}, nil
		}
		userIDs = make([]string, 0, len(users))
		for _, u := range users {
			userIDs = append(userIDs, u.ID)
		}
	}

	filterMap, err := auditFilterToMap(p, userIDs)
	if err != nil {
		return nil, err
	}

	total, err := s.repo.Count(ctx, filterMap)
	if err != nil {
		return nil, err
	}

	logs, err := s.repo.List(ctx, filterMap, p.Skip, p.Limit)
	if err != nil {
		return nil, err
	}

	s.enrichUserEmails(ctx, logs)

	return &ListResult{Logs: logs, Total: total}, nil
}

// enrichUserEmails replaces each log's user ID with the user's email.
// Missing/deleted users keep their original ID, and a lookup failure is
// non-fatal (logs are still returned with raw IDs).
func (s *Service) enrichUserEmails(ctx context.Context, logs []model.AuditLog) {
	if len(logs) == 0 || s.userRepo == nil {
		return
	}
	idSet := make(map[string]struct{}, len(logs))
	for _, l := range logs {
		if l.UserID != "" {
			idSet[l.UserID] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	users, err := s.userRepo.FindByIDs(ctx, ids)
	if err != nil {
		log.Printf("audit: enrich user emails: %v", err)
		return
	}
	emailMap := make(map[string]string, len(users))
	for _, u := range users {
		if u.Username != "" {
			emailMap[u.ID] = u.Username
		}
	}
	for i := range logs {
		if email, ok := emailMap[logs[i].UserID]; ok {
			logs[i].UserID = email
		}
	}
}

func auditFilterToMap(p ListParams, userIDs []string) (map[string]interface{}, error) {
	m := map[string]interface{}{}
	if p.Action != "" {
		m["action"] = p.Action
	}
	if len(userIDs) > 0 {
		m["user_id"] = map[string]interface{}{"$in": userIDs}
	}
	dateFilter, err := buildDateFilter(p.Start, p.End)
	if err != nil {
		return nil, err
	}
	if len(dateFilter) > 0 {
		m["created_at"] = dateFilter
	}
	return m, nil
}

func buildDateFilter(start, end string) (map[string]interface{}, error) {
	if start == "" && end == "" {
		return nil, nil
	}
	m := map[string]interface{}{}
	if start != "" {
		t, err := time.Parse("2006-01-02", start)
		if err != nil {
			return nil, fmt.Errorf("invalid start date %q: must be YYYY-MM-DD", start)
		}
		m["$gte"] = t
	}
	if end != "" {
		t, err := time.Parse("2006-01-02", end)
		if err != nil {
			return nil, fmt.Errorf("invalid end date %q: must be YYYY-MM-DD", end)
		}
		m["$lt"] = t.Add(24 * time.Hour)
	}
	return m, nil
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
