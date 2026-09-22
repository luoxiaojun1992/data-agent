package audit

import (
	"errors"
	"regexp"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	"github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

func TestNewService(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	userRepo := mocks.NewUserRepository(t)
	s := NewService(repo, userRepo)
	if s == nil {
		t.Fatal("NewService should not return nil")
	}
	if s.repo != repo {
		t.Error("Service.repo should be the injected repository")
	}
	if s.userRepo != userRepo {
		t.Error("Service.userRepo should be the injected user repository")
	}
}

// ── List: no filters, system admin ──

func TestList_Success_NoFilters_SystemAdmin(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(0), nil)
	repo.On("List", mock.Anything, mock.Anything, int64(0), int64(20)).Return([]model.AuditLog{}, nil)

	result, err := NewService(repo, nil).List(ListParams{IsSystemAdmin: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil ListResult")
	}
	if result.Logs == nil {
		t.Error("expected non-nil Logs slice")
	}
	if result.Total != 0 {
		t.Errorf("Total: got %d, want 0", result.Total)
	}
}

// ── Visibility (D9) ──

func TestList_Visibility_NonSystemAdmin(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.MatchedBy(hasVisibilityOr("u1"))).Return(int64(0), nil)
	repo.On("List", mock.Anything, mock.MatchedBy(hasVisibilityOr("u1")), int64(0), int64(20)).Return([]model.AuditLog{}, nil)

	result, err := NewService(repo, nil).List(ListParams{ViewerUserID: "u1", IsSystemAdmin: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("Total: got %d, want 0", result.Total)
	}
}

func TestList_Visibility_SystemAdmin_NoOr(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.MatchedBy(hasNoVisibilityOr())).Return(int64(0), nil)
	repo.On("List", mock.Anything, mock.MatchedBy(hasNoVisibilityOr()), int64(0), int64(20)).Return([]model.AuditLog{}, nil)

	_, err := NewService(repo, nil).List(ListParams{ViewerUserID: "u1", IsSystemAdmin: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── q email search (D1) ──

func TestList_Q_EmailSearch(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	userRepo := mocks.NewUserRepository(t)

	userRepo.On("SearchByEmail", mock.Anything, "admin", 10).
		Return([]model.User{{ID: "u1", Username: "admin@example.com"}}, nil)
	repo.On("Count", mock.Anything, mock.MatchedBy(hasInFilter("user_id", "u1"))).Return(int64(5), nil)
	repo.On("List", mock.Anything, mock.MatchedBy(hasInFilter("user_id", "u1")), int64(0), int64(50)).Return([]model.AuditLog{}, nil)

	result, err := NewService(repo, userRepo).List(ListParams{
		Q:             "admin",
		Page:          1,
		PageSize:      50,
		IsSystemAdmin: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 5 {
		t.Errorf("Total: got %d, want 5", result.Total)
	}
}

func TestList_Q_NoMatch(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	userRepo := mocks.NewUserRepository(t)

	userRepo.On("SearchByEmail", mock.Anything, "nobody", 10).Return([]model.User{}, nil)

	result, err := NewService(repo, userRepo).List(ListParams{Q: "nobody"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("Total: got %d, want 0", result.Total)
	}
	if len(result.Logs) != 0 {
		t.Errorf("Logs: got %d, want 0", len(result.Logs))
	}
	repo.AssertNotCalled(t, "Count", mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "List", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestList_Q_EmailSearchError(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	userRepo := mocks.NewUserRepository(t)
	userRepo.On("SearchByEmail", mock.Anything, "admin", 10).Return(nil, errors.New("search failed"))

	_, err := NewService(repo, userRepo).List(ListParams{Q: "admin"})
	if err == nil {
		t.Fatal("expected error for email search failure")
	}
}

// ── path fuzzy search (D2) ──

func TestList_PathFilter(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.MatchedBy(hasActionRegex("sessions"))).Return(int64(1), nil)
	repo.On("List", mock.Anything, mock.MatchedBy(hasActionRegex("sessions")), int64(0), int64(20)).Return([]model.AuditLog{}, nil)

	_, err := NewService(repo, nil).List(ListParams{Path: "sessions", IsSystemAdmin: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestList_PathRegexEscaped(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.MatchedBy(hasActionRegex("a.b*c"))).Return(int64(0), nil)
	repo.On("List", mock.Anything, mock.MatchedBy(hasActionRegex("a.b*c")), int64(0), int64(20)).Return([]model.AuditLog{}, nil)

	_, err := NewService(repo, nil).List(ListParams{Path: "a.b*c", IsSystemAdmin: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── status_class (D3) ──

func TestList_StatusClassFilter(t *testing.T) {
	cases := []struct {
		class string
		gte   int
		lt    int
	}{
		{"1xx", 100, 200},
		{"2xx", 200, 300},
		{"3xx", 300, 400},
		{"4xx", 400, 500},
		{"5xx", 500, 600},
	}
	for _, c := range cases {
		t.Run(c.class, func(t *testing.T) {
			repo := mocks.NewAuditRepository(t)
			repo.On("Count", mock.Anything, mock.MatchedBy(hasStatusRange(c.gte, c.lt))).Return(int64(0), nil)
			repo.On("List", mock.Anything, mock.MatchedBy(hasStatusRange(c.gte, c.lt)), int64(0), int64(20)).Return([]model.AuditLog{}, nil)

			_, err := NewService(repo, nil).List(ListParams{StatusClass: c.class, IsSystemAdmin: true})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestList_StatusClassInvalid(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	_, err := NewService(repo, nil).List(ListParams{StatusClass: "7xx"})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

// ── date filter (D6/D8) ──

func TestList_DateFilter(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.MatchedBy(hasCreatedAtRange())).Return(int64(0), nil)
	repo.On("List", mock.Anything, mock.MatchedBy(hasCreatedAtRange()), int64(0), int64(20)).Return([]model.AuditLog{}, nil)

	_, err := NewService(repo, nil).List(ListParams{Start: "2024-01-01", End: "2024-12-31", IsSystemAdmin: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestList_DateInvalidFormat(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	_, err := NewService(repo, nil).List(ListParams{Start: "invalid-date"})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestList_DateStartAfterEnd(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	_, err := NewService(repo, nil).List(ListParams{Start: "2024-12-31", End: "2024-01-01"})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

// ── action_desc mapping (D4) ──

func TestList_ActionDesc_Hit(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	logs := []model.AuditLog{{ID: "l1", Action: "POST /api/v1/chat"}}
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(1), nil)
	repo.On("List", mock.Anything, mock.Anything, int64(0), int64(20)).Return(logs, nil)

	result, err := NewService(repo, nil).List(ListParams{IsSystemAdmin: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Logs[0].ActionDesc != "Chat 对话" {
		t.Errorf("ActionDesc: got %q, want %q", result.Logs[0].ActionDesc, "Chat 对话")
	}
}

func TestList_ActionDesc_Miss_Fallback(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	logs := []model.AuditLog{{ID: "l1", Action: "POST /api/v1/unknown"}}
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(1), nil)
	repo.On("List", mock.Anything, mock.Anything, int64(0), int64(20)).Return(logs, nil)

	result, err := NewService(repo, nil).List(ListParams{IsSystemAdmin: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Logs[0].ActionDesc != "POST /api/v1/unknown" {
		t.Errorf("ActionDesc fallback: got %q, want raw action", result.Logs[0].ActionDesc)
	}
}

// ── enrich emails ──

func TestList_EnrichEmails(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	userRepo := mocks.NewUserRepository(t)

	logs := []model.AuditLog{
		{ID: "l1", UserID: "u1"},
		{ID: "l2", UserID: "u2"},
		{ID: "l3", UserID: "u1"},
	}
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(3), nil)
	repo.On("List", mock.Anything, mock.Anything, int64(0), int64(20)).Return(logs, nil)
	userRepo.On("FindByIDs", mock.Anything, mock.MatchedBy(idsContain("u1", "u2"))).
		Return([]model.User{
			{ID: "u1", Username: "a@example.com"},
			{ID: "u2", Username: "b@example.com"},
		}, nil)

	result, err := NewService(repo, userRepo).List(ListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Logs[0].UserID != "a@example.com" {
		t.Errorf("log[0] user: got %q, want email", result.Logs[0].UserID)
	}
	if result.Logs[1].UserID != "b@example.com" {
		t.Errorf("log[1] user: got %q, want email", result.Logs[1].UserID)
	}
	if result.Logs[2].UserID != "a@example.com" {
		t.Errorf("log[2] user: got %q, want email", result.Logs[2].UserID)
	}
}

// ── errors ──

func TestList_CountError(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(0), errors.New("count failed"))

	_, err := NewService(repo, nil).List(ListParams{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestList_FindError(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(0), nil)
	repo.On("List", mock.Anything, mock.Anything, int64(0), int64(20)).Return(([]model.AuditLog)(nil), errors.New("list failed"))

	_, err := NewService(repo, nil).List(ListParams{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── pagination normalization ──

func TestList_DefaultPagination(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(0), nil)
	repo.On("List", mock.Anything, mock.Anything, int64(0), int64(20)).Return([]model.AuditLog{}, nil)

	result, err := NewService(repo, nil).List(ListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil ListResult")
	}
}

func TestList_PageTwo_Skip(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.Anything).Return(int64(0), nil)
	repo.On("List", mock.Anything, mock.Anything, int64(20), int64(20)).Return([]model.AuditLog{}, nil)

	_, err := NewService(repo, nil).List(ListParams{Page: 2, PageSize: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── Export ──

func TestExport_SharesFilter_AndActionDesc(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	userRepo := mocks.NewUserRepository(t)
	userRepo.On("SearchByEmail", mock.Anything, "admin@example.com", 10).
		Return([]model.User{{ID: "u1"}}, nil)

	logs := []model.AuditLog{
		{ID: "l1", Action: "POST /api/v1/chat"},
		{ID: "l2", Action: "POST /api/v1/users"},
	}
	// Assert BOTH the q→$in filter and the non-system-admin visibility $or.
	filter := func(m map[string]interface{}) bool {
		return hasInFilter("user_id", "u1")(m) && hasVisibilityOr("v1")(m)
	}
	repo.On("List", mock.Anything, mock.MatchedBy(filter), int64(0), int64(100)).Return(logs, nil)

	out, err := NewService(repo, userRepo).Export(ExportParams{
		Q:             "admin@example.com",
		Limit:         100,
		ViewerUserID:  "v1",
		IsSystemAdmin: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len: got %d, want 2", len(out))
	}
	if out[0].ActionDesc != "Chat 对话" {
		t.Errorf("ActionDesc: got %q, want %q", out[0].ActionDesc, "Chat 对话")
	}
	if out[1].ActionDesc != "创建用户" {
		t.Errorf("ActionDesc: got %q, want %q", out[1].ActionDesc, "创建用户")
	}
}

func TestExport_InvalidStatusClass(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	_, err := NewService(repo, nil).Export(ExportParams{StatusClass: "9xx"})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestExport_InvalidDate(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	_, err := NewService(repo, nil).Export(ExportParams{Start: "bad-date"})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

// ── coverage: validation + date single-bound + nil user repo ──

func TestValidationError_Error(t *testing.T) {
	ve := NewValidationError("bad %d", 42)
	if ve.Error() != "bad 42" {
		t.Errorf("Error: got %q, want %q", ve.Error(), "bad 42")
	}
}

func TestList_StartOnly(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.MatchedBy(hasCreatedAtGteOnly())).Return(int64(0), nil)
	repo.On("List", mock.Anything, mock.MatchedBy(hasCreatedAtGteOnly()), int64(0), int64(20)).Return([]model.AuditLog{}, nil)

	_, err := NewService(repo, nil).List(ListParams{Start: "2024-01-01", IsSystemAdmin: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestList_EndOnly(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("Count", mock.Anything, mock.MatchedBy(hasCreatedAtLtOnly())).Return(int64(0), nil)
	repo.On("List", mock.Anything, mock.MatchedBy(hasCreatedAtLtOnly()), int64(0), int64(20)).Return([]model.AuditLog{}, nil)

	_, err := NewService(repo, nil).List(ListParams{End: "2024-12-31", IsSystemAdmin: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestList_Q_NilUserRepo(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	_, err := NewService(repo, nil).List(ListParams{Q: "admin"})
	if err == nil {
		t.Fatal("expected error when user repo is nil and q is set")
	}
}

func TestExport_DefaultLimit(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	repo.On("List", mock.Anything, mock.Anything, int64(0), int64(5000)).Return([]model.AuditLog{}, nil)

	out, err := NewService(repo, nil).Export(ExportParams{Limit: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Error("expected non-nil output")
	}
}

func TestExport_QNoMatch(t *testing.T) {
	repo := mocks.NewAuditRepository(t)
	userRepo := mocks.NewUserRepository(t)
	userRepo.On("SearchByEmail", mock.Anything, "nobody", 10).Return([]model.User{}, nil)

	out, err := NewService(repo, userRepo).Export(ExportParams{Q: "nobody"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("len: got %d, want 0", len(out))
	}
	repo.AssertNotCalled(t, "List", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// ── describeAction unit ──

func TestDescribeAction(t *testing.T) {
	if got := describeAction("POST /api/v1/chat"); got != "Chat 对话" {
		t.Errorf("hit: got %q", got)
	}
	if got := describeAction("POST /api/v1/not-a-route"); got != "POST /api/v1/not-a-route" {
		t.Errorf("miss: got %q, want raw", got)
	}
	if got := describeAction(""); got != "" {
		t.Errorf("empty: got %q", got)
	}
}

// ── helpers ──

// hasInFilter asserts a filter map has `field` set to {$in: [...]} containing want.
func hasInFilter(field, want string) func(map[string]interface{}) bool {
	return func(m map[string]interface{}) bool {
		v, ok := m[field]
		if !ok {
			return false
		}
		inner, ok := v.(map[string]interface{})
		if !ok {
			return false
		}
		in, ok := inner["$in"].([]string)
		if !ok {
			return false
		}
		for _, s := range in {
			if s == want {
				return true
			}
		}
		return false
	}
}

// hasVisibilityOr asserts the $or visibility clause matches the D9 shape.
func hasVisibilityOr(viewer string) func(map[string]interface{}) bool {
	return func(m map[string]interface{}) bool {
		v, ok := m["$or"]
		if !ok {
			return false
		}
		or, ok := v.([]map[string]interface{})
		if !ok || len(or) != 3 {
			return false
		}
		if or[0]["user_id"] != viewer {
			return false
		}
		if or[1]["user_id"] != "" {
			return false
		}
		inner, ok := or[2]["user_id"].(map[string]interface{})
		if !ok {
			return false
		}
		ev, ok := inner["$exists"].(bool)
		return ok && !ev
	}
}

// hasNoVisibilityOr asserts the filter has NO $or clause (system admin).
func hasNoVisibilityOr() func(map[string]interface{}) bool {
	return func(m map[string]interface{}) bool {
		_, ok := m["$or"]
		return !ok
	}
}

// hasActionRegex asserts the action filter is {$regex: QuoteMeta(want), $options: "i"}.
func hasActionRegex(want string) func(map[string]interface{}) bool {
	return func(m map[string]interface{}) bool {
		v, ok := m["action"]
		if !ok {
			return false
		}
		inner, ok := v.(map[string]interface{})
		if !ok {
			return false
		}
		re, ok := inner["$regex"].(string)
		if !ok {
			return false
		}
		opt, _ := inner["$options"].(string)
		return re == regexp.QuoteMeta(want) && opt == "i"
	}
}

// hasStatusRange asserts the status_code filter is {$gte: gte, $lt: lt}.
func hasStatusRange(gte, lt int) func(map[string]interface{}) bool {
	return func(m map[string]interface{}) bool {
		v, ok := m["status_code"]
		if !ok {
			return false
		}
		inner, ok := v.(map[string]interface{})
		if !ok {
			return false
		}
		g, ok1 := inner["$gte"].(int)
		l, ok2 := inner["$lt"].(int)
		return ok1 && ok2 && g == gte && l == lt
	}
}

// hasCreatedAtRange asserts the created_at filter has both $gte and $lt.
func hasCreatedAtRange() func(map[string]interface{}) bool {
	return func(m map[string]interface{}) bool {
		v, ok := m["created_at"]
		if !ok {
			return false
		}
		inner, ok := v.(map[string]interface{})
		if !ok {
			return false
		}
		_, hasGte := inner["$gte"]
		_, hasLt := inner["$lt"]
		return hasGte && hasLt
	}
}

// hasCreatedAtGteOnly asserts the created_at filter has $gte but NOT $lt.
func hasCreatedAtGteOnly() func(map[string]interface{}) bool {
	return func(m map[string]interface{}) bool {
		v, ok := m["created_at"]
		if !ok {
			return false
		}
		inner, ok := v.(map[string]interface{})
		if !ok {
			return false
		}
		_, hasGte := inner["$gte"]
		_, hasLt := inner["$lt"]
		return hasGte && !hasLt
	}
}

// hasCreatedAtLtOnly asserts the created_at filter has $lt but NOT $gte.
func hasCreatedAtLtOnly() func(map[string]interface{}) bool {
	return func(m map[string]interface{}) bool {
		v, ok := m["created_at"]
		if !ok {
			return false
		}
		inner, ok := v.(map[string]interface{})
		if !ok {
			return false
		}
		_, hasGte := inner["$gte"]
		_, hasLt := inner["$lt"]
		return !hasGte && hasLt
	}
}

// idsContain asserts the []string arg contains all wanted IDs.
func idsContain(want ...string) func([]string) bool {
	return func(ids []string) bool {
		seen := map[string]bool{}
		for _, id := range ids {
			seen[id] = true
		}
		for _, w := range want {
			if !seen[w] {
				return false
			}
		}
		return true
	}
}

// ensure repository.UserRepository import is used (kept for interface clarity).
var _ repository.UserRepository = (*mocks.UserRepository)(nil)
