package audit

import (
	"context"
	"fmt"
	"log"
	"regexp"
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

// ValidationError marks invalid user input that must map to HTTP 400 (SPEC-102
// D6). Any other error returned by the service is an internal failure (500).
type ValidationError struct{ msg string }

func (e *ValidationError) Error() string { return e.msg }

// NewValidationError constructs a ValidationError with a formatted message.
func NewValidationError(format string, args ...interface{}) *ValidationError {
	return &ValidationError{msg: fmt.Sprintf(format, args...)}
}

// ListParams are the filter parameters for listing audit logs (SPEC-102 §5.1).
// q = operator email keyword; path = API path fuzzy match; status_class = HTTP
// status class (1xx~5xx); page/page_size = DB-level pagination; viewer fields
// drive the row-level visibility filter (D9).
type ListParams struct {
	Q             string
	Path          string
	StatusClass   string
	Start         string
	End           string
	Page          int
	PageSize      int
	ViewerUserID  string
	IsSystemAdmin bool
}

// ListResult contains the audit log list and total count.
type ListResult struct {
	Logs  []model.AuditLog `json:"logs"`
	Total int64            `json:"total"`
}

// List returns audit logs matching the filter params.
//
// All filters (q/path/status_class/date/visibility) are AND-composed at the DB
// layer. The q keyword resolves to a set of user IDs via email fuzzy search
// (top 10); path matches the raw action field; status_class maps to a status
// code range. Non-system-admin viewers only see their own logs plus logs with
// no user association (D9). Each returned log is enriched with its operator
// email and a human-friendly action_desc.
func (s *Service) List(p ListParams) (*ListResult, error) {
	page, pageSize := normalizePage(p.Page, p.PageSize)
	ctx := context.Background()

	if err := validateStatusClass(p.StatusClass); err != nil {
		return nil, err
	}
	if _, err := buildDateFilter(p.Start, p.End); err != nil {
		return nil, err
	}

	userIDs, empty, err := s.resolveQ(ctx, p.Q)
	if err != nil {
		return nil, err
	}
	if empty {
		return &ListResult{Logs: []model.AuditLog{}, Total: 0}, nil
	}

	filterMap, err := s.buildFilter(p, userIDs)
	if err != nil {
		return nil, err
	}

	total, err := s.repo.Count(ctx, filterMap)
	if err != nil {
		return nil, err
	}

	skip := int64((page - 1) * pageSize)
	logs, err := s.repo.List(ctx, filterMap, skip, int64(pageSize))
	if err != nil {
		return nil, err
	}

	s.enrichUserEmails(ctx, logs)
	s.applyActionDesc(logs)

	return &ListResult{Logs: logs, Total: total}, nil
}

// ExportParams defines the export request (SPEC-102 D6). The viewer fields are
// injected by the handler (not deserialized from the client body).
type ExportParams struct {
	Q             string `json:"q"`
	Path          string `json:"path"`
	StatusClass   string `json:"status_class"`
	Start         string `json:"start"`
	End           string `json:"end"`
	Limit         int    `json:"limit"`
	Format        string `json:"format"`
	ViewerUserID  string `json:"-"`
	IsSystemAdmin bool   `json:"-"`
}

// Export returns all audit logs matching the shared filter + visibility logic,
// up to the requested limit (CSV download). The list and export paths share the
// same filter assembly so results stay consistent (D9/D6).
func (s *Service) Export(p ExportParams) ([]model.AuditLog, error) {
	ctx := context.Background()

	if err := validateStatusClass(p.StatusClass); err != nil {
		return nil, err
	}
	if _, err := buildDateFilter(p.Start, p.End); err != nil {
		return nil, err
	}

	userIDs, empty, err := s.resolveQ(ctx, p.Q)
	if err != nil {
		return nil, err
	}
	if empty {
		return []model.AuditLog{}, nil
	}

	// Reuse the same filter assembly as List (minus pagination).
	lp := ListParams{
		Q:             p.Q,
		Path:          p.Path,
		StatusClass:   p.StatusClass,
		Start:         p.Start,
		End:           p.End,
		ViewerUserID:  p.ViewerUserID,
		IsSystemAdmin: p.IsSystemAdmin,
	}
	filterMap, err := s.buildFilter(lp, userIDs)
	if err != nil {
		return nil, err
	}

	limit := int64(p.Limit)
	if limit <= 0 {
		limit = 5000
	}

	logs, err := s.repo.List(ctx, filterMap, 0, limit)
	if err != nil {
		return nil, err
	}

	s.enrichUserEmails(ctx, logs)
	s.applyActionDesc(logs)
	return logs, nil
}

// resolveQ resolves the q keyword to a set of user IDs via email fuzzy search
// (top 10). empty == true means the keyword matched no user → caller returns an
// empty result without touching the audit collection (D1).
func (s *Service) resolveQ(ctx context.Context, q string) (userIDs []string, empty bool, err error) {
	if q == "" {
		return nil, false, nil
	}
	if s.userRepo == nil {
		return nil, false, fmt.Errorf("user repository not available")
	}
	users, err := s.userRepo.SearchByEmail(ctx, q, 10)
	if err != nil {
		return nil, false, fmt.Errorf("search users by email: %w", err)
	}
	if len(users) == 0 {
		return nil, true, nil
	}
	userIDs = make([]string, 0, len(users))
	for _, u := range users {
		userIDs = append(userIDs, u.ID)
	}
	return userIDs, false, nil
}

// buildFilter assembles the MongoDB filter: path ($regex on action), status
// class range, q user IDs ($in), date range, and row-level visibility ($or).
func (s *Service) buildFilter(p ListParams, userIDs []string) (map[string]interface{}, error) {
	m := map[string]interface{}{}
	if p.Path != "" {
		m["action"] = map[string]interface{}{
			"$regex":   regexp.QuoteMeta(p.Path),
			"$options": "i",
		}
	}
	if p.StatusClass != "" {
		rng, err := statusClassRange(p.StatusClass)
		if err != nil {
			return nil, err
		}
		m["status_code"] = rng
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
	if !p.IsSystemAdmin {
		// Row-level visibility (D9): own logs + logs with no user association.
		m["$or"] = []map[string]interface{}{
			{"user_id": p.ViewerUserID},
			{"user_id": ""},
			{"user_id": map[string]interface{}{"$exists": false}},
		}
	}
	return m, nil
}

// statusClassRange maps a status class enum to a [X00, (X+1)00) range (D3).
func statusClassRange(sc string) (map[string]interface{}, error) {
	switch sc {
	case "1xx":
		return map[string]interface{}{"$gte": 100, "$lt": 200}, nil
	case "2xx":
		return map[string]interface{}{"$gte": 200, "$lt": 300}, nil
	case "3xx":
		return map[string]interface{}{"$gte": 300, "$lt": 400}, nil
	case "4xx":
		return map[string]interface{}{"$gte": 400, "$lt": 500}, nil
	case "5xx":
		return map[string]interface{}{"$gte": 500, "$lt": 600}, nil
	default:
		return nil, NewValidationError("invalid status_class %q: must be one of 1xx/2xx/3xx/4xx/5xx", sc)
	}
}

// validateStatusClass returns a ValidationError when sc is non-empty but not a
// valid status class enum.
func validateStatusClass(sc string) error {
	if sc == "" {
		return nil
	}
	_, err := statusClassRange(sc)
	return err
}

// buildDateFilter parses start/end (YYYY-MM-DD) into a created_at range.
// Returns a ValidationError on malformed dates or start > end (D6/D8: no
// clamping of the range itself — any historical start is accepted).
func buildDateFilter(start, end string) (map[string]interface{}, error) {
	if start == "" && end == "" {
		return nil, nil
	}
	m := map[string]interface{}{}
	var st, en time.Time
	var err error
	if start != "" {
		st, err = time.Parse("2006-01-02", start)
		if err != nil {
			return nil, NewValidationError("invalid start date %q: must be YYYY-MM-DD", start)
		}
		m["$gte"] = st
	}
	if end != "" {
		en, err = time.Parse("2006-01-02", end)
		if err != nil {
			return nil, NewValidationError("invalid end date %q: must be YYYY-MM-DD", end)
		}
		m["$lt"] = en.Add(24 * time.Hour)
	}
	if !st.IsZero() && !en.IsZero() && st.After(en) {
		return nil, NewValidationError("start date must not be after end date")
	}
	return m, nil
}

// normalizePage clamps page/page_size for direct service callers. The handler
// already rejects invalid values with 400; this is defense-in-depth only.
func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}

// applyActionDesc fills each log's ActionDesc from the hardcoded mapping.
func (s *Service) applyActionDesc(logs []model.AuditLog) {
	for i := range logs {
		logs[i].ActionDesc = describeAction(logs[i].Action)
	}
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
