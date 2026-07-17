package skill

import (
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	skilldomain "github.com/luoxiaojun1992/data-agent/internal/domain/skill"
	sqlpkg "github.com/luoxiaojun1992/data-agent/internal/logic/sql"
)

func TestSQLExecutor_Name(t *testing.T) {
	s := &SQLExecutor{}
	if got := s.Name(); got != "sql_executor" {
		t.Errorf("Name() = %q, want %q", got, "sql_executor")
	}
}

func TestSQLExecutor_Description(t *testing.T) {
	s := &SQLExecutor{}
	desc := s.Description()
	if desc == "" {
		t.Error("Description() should not be empty")
	}
}

func TestSQLExecutor_Parameters(t *testing.T) {
	s := &SQLExecutor{}
	params := s.Parameters()
	if len(params) < 1 {
		t.Fatal("Parameters() should return at least 1 parameter")
	}
	if params[0].Name != "query" {
		t.Errorf("first param name = %q, want %q", params[0].Name, "query")
	}
	if !params[0].Required {
		t.Error("'query' parameter should be required")
	}
}

func TestSQLExecutor_Permissions(t *testing.T) {
	s := &SQLExecutor{}
	perms := s.Permissions()
	if len(perms) == 0 {
		t.Error("Permissions() should not be empty")
	}
	found := false
	for _, p := range perms {
		if p == "skill:sql_executor" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Permissions() should contain 'skill:sql_executor'")
	}
}

func TestSQLExecutor_RateLimit(t *testing.T) {
	s := &SQLExecutor{}
	rl := s.RateLimit()
	if rl == nil {
		t.Fatal("RateLimit() should not be nil")
	}
	if rl.MaxRequests != 30 {
		t.Errorf("MaxRequests = %d, want 30", rl.MaxRequests)
	}
	if rl.WindowSec != 60 {
		t.Errorf("WindowSec = %d, want 60", rl.WindowSec)
	}
}

func TestSQLExecutor_Execute_MissingQuery(t *testing.T) {
	s := &SQLExecutor{}
	ctx := skilldomain.SkillContext{}
	result, err := s.Execute(ctx, map[string]any{})
	if err == nil {
		t.Error("Execute() should return error for missing 'query' parameter")
	}
	if result != nil {
		t.Error("Execute() should return nil result on error")
	}
}

func TestSQLExecutor_Execute_EmptyQuery(t *testing.T) {
	s := &SQLExecutor{}
	ctx := skilldomain.SkillContext{}
	result, err := s.Execute(ctx, map[string]any{"query": ""})
	if err == nil {
		t.Error("Execute() should return error for empty 'query'")
	}
	if result != nil {
		t.Error("Execute() should return nil result on error")
	}
}

func TestSQLExecutor_Execute_InvalidQueryType(t *testing.T) {
	s := &SQLExecutor{}
	ctx := skilldomain.SkillContext{}
	result, err := s.Execute(ctx, map[string]any{"query": 123})
	if err == nil {
		t.Error("Execute() should return error when 'query' is not a string")
	}
	if result != nil {
		t.Error("Execute() should return nil result on error")
	}
}

func TestSQLExecutor_Execute_RejectedQuery(t *testing.T) {
	s := &SQLExecutor{}
	ctx := skilldomain.SkillContext{UserID: "user1", Role: "user"}
	result, err := s.Execute(ctx, map[string]any{
		"query": "DROP TABLE users",
	})
	if err == nil {
		t.Error("Execute() should return error for rejected SQL")
	}
	if result != nil {
		t.Error("Execute() should return nil result on error")
	}
}

func TestSQLExecutor_Execute_Success(t *testing.T) {
	patches := gomonkey.ApplyFunc(sqlpkg.Validate, func(sql string, params []interface{}) *sqlpkg.ValidationResult {
		return &sqlpkg.ValidationResult{Allowed: true, Reason: "VALID SELECT"}
	})
	defer patches.Reset()

	s := &SQLExecutor{}
	ctx := skilldomain.SkillContext{UserID: "user1", Role: "user"}
	result, err := s.Execute(ctx, map[string]any{
		"query": "SELECT * FROM users WHERE id = ?",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}
	if resultMap["status"] != "validated" {
		t.Errorf("status = %v, want 'validated'", resultMap["status"])
	}
	if resultMap["query"] != "SELECT * FROM users WHERE id = ?" {
		t.Errorf("query = %v", resultMap["query"])
	}
}

func TestSQLExecutor_Execute_WithBindParams(t *testing.T) {
	patches := gomonkey.ApplyFunc(sqlpkg.Validate, func(sql string, params []interface{}) *sqlpkg.ValidationResult {
		return &sqlpkg.ValidationResult{Allowed: true, Reason: "Parameterized query"}
	})
	defer patches.Reset()

	s := &SQLExecutor{}
	ctx := skilldomain.SkillContext{UserID: "user1", Role: "user"}
	result, err := s.Execute(ctx, map[string]any{
		"query":  "SELECT * FROM users WHERE status = ?",
		"params": []interface{}{"active"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	resultMap := result.(map[string]any)
	if resultMap["status"] != "validated" {
		t.Errorf("status = %v, want 'validated'", resultMap["status"])
	}
}

func TestSQLExecutor_Execute_ValidatorRejected(t *testing.T) {
	patches := gomonkey.ApplyFunc(sqlpkg.Validate, func(sql string, params []interface{}) *sqlpkg.ValidationResult {
		return &sqlpkg.ValidationResult{Allowed: false, Reason: "SQL injection detected"}
	})
	defer patches.Reset()

	s := &SQLExecutor{}
	ctx := skilldomain.SkillContext{UserID: "user1", Role: "user"}
	result, err := s.Execute(ctx, map[string]any{
		"query": "1; DROP TABLE users; --",
	})
	if err == nil {
		t.Error("Execute() should return error for validator-rejected query")
	}
	if result != nil {
		t.Error("Execute() should return nil result on error")
	}
}
