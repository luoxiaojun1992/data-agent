package skill

import (
	"context"
	"errors"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	skilldomain "github.com/luoxiaojun1992/data-agent/internal/domain/skill"
	knowledgepkg "github.com/luoxiaojun1992/data-agent/internal/service/knowledge"
)

func TestNewKnowledgeSearch(t *testing.T) {
	kb := &knowledgepkg.Service{}
	ks := NewKnowledgeSearch(kb)
	if ks == nil {
		t.Fatal("NewKnowledgeSearch should not return nil")
	}
	if ks.kbService != kb {
		t.Error("kbService should be the passed-in service")
	}
}

func TestNewKnowledgeSearch_NilService(t *testing.T) {
	ks := NewKnowledgeSearch(nil)
	if ks == nil {
		t.Fatal("NewKnowledgeSearch should not return nil even with nil service")
	}
}

func TestKnowledgeSearch_Name(t *testing.T) {
	ks := NewKnowledgeSearch(nil)
	if ks.Name() != "knowledge_search" {
		t.Errorf("Name = %q, want %q", ks.Name(), "knowledge_search")
	}
}

func TestKnowledgeSearch_Description(t *testing.T) {
	ks := NewKnowledgeSearch(nil)
	desc := ks.Description()
	if desc == "" {
		t.Error("Description should not be empty")
	}
	if !containsString(desc, "knowledge") && !containsString(desc, "search") {
		t.Errorf("Description = %q, should reference knowledge/search", desc)
	}
}

func TestKnowledgeSearch_Parameters(t *testing.T) {
	ks := NewKnowledgeSearch(nil)
	params := ks.Parameters()
	if len(params) != 3 {
		t.Fatalf("got %d parameters, want 3", len(params))
	}

	// query (index 0) is required
	if params[0].Name != "query" {
		t.Errorf("params[0].Name = %q, want %q", params[0].Name, "query")
	}
	if params[0].Type != "string" {
		t.Errorf("params[0].Type = %q, want %q", params[0].Type, "string")
	}
	if !params[0].Required {
		t.Error("params[0].Required should be true")
	}

	// top_k (index 1) is optional
	if params[1].Name != "top_k" {
		t.Errorf("params[1].Name = %q, want %q", params[1].Name, "top_k")
	}
	if params[1].Type != "integer" {
		t.Errorf("params[1].Type = %q, want %q", params[1].Type, "integer")
	}
	if params[1].Required {
		t.Error("params[1].Required should be false")
	}

	// role (index 2) is optional
	if params[2].Name != "role" {
		t.Errorf("params[2].Name = %q, want %q", params[2].Name, "role")
	}
	if params[2].Type != "string" {
		t.Errorf("params[2].Type = %q, want %q", params[2].Type, "string")
	}
	if params[2].Required {
		t.Error("params[2].Required should be false")
	}
}

func TestKnowledgeSearch_Permissions(t *testing.T) {
	ks := NewKnowledgeSearch(nil)
	perms := ks.Permissions()
	if len(perms) != 1 {
		t.Fatalf("got %d permissions, want 1", len(perms))
	}
	if perms[0] != "skill:knowledge_search" {
		t.Errorf("perms[0] = %q, want %q", perms[0], "skill:knowledge_search")
	}
}

func TestKnowledgeSearch_RateLimit(t *testing.T) {
	ks := NewKnowledgeSearch(nil)
	rl := ks.RateLimit()
	if rl == nil {
		t.Fatal("RateLimit should not be nil")
	}
	if rl.MaxRequests != 30 {
		t.Errorf("MaxRequests = %d, want 30", rl.MaxRequests)
	}
	if rl.WindowSec != 60 {
		t.Errorf("WindowSec = %d, want 60", rl.WindowSec)
	}
}

// ========== gomonkey-based tests for Execute ==========

func TestKnowledgeSearch_Execute_Success(t *testing.T) {
	mockResults := []knowledge.SearchResult{
		{ChunkID: "c1", DocID: "d1", DocTitle: "Test Document", Content: "This is the search result content.", Score: 0.95, Source: "fulltext"},
		{ChunkID: "c2", DocID: "d1", DocTitle: "Test Document", Content: "Another chunk of content.", Score: 0.80, Source: "fulltext"},
	}

	patches := gomonkey.ApplyMethodFunc(&knowledgepkg.Service{}, "Search",
		func(userID, query string, topK int, role string) ([]knowledge.SearchResult, error) {
			// Verify correct parameters are passed through
			if userID != "user_abc" {
				t.Errorf("Search userID = %q, want %q", userID, "user_abc")
			}
			if query != "test query" {
				t.Errorf("Search query = %q, want %q", query, "test query")
			}
			if topK != 5 {
				t.Errorf("Search topK = %d, want %d", topK, 5)
			}
			return mockResults, nil
		})
	defer patches.Reset()

	ks := NewKnowledgeSearch(&knowledgepkg.Service{})
	ctx := skilldomain.NewSkillContext(context.Background(), "session1", "user_abc", "task1", "trace1", "user")

	result, err := ks.Execute(ctx, map[string]any{
		"query": "test query",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}
	if resultMap["query"] != "test query" {
		t.Errorf("query = %v, want %q", resultMap["query"], "test query")
	}
	if count, ok := resultMap["count"].(int); !ok || count != 2 {
		t.Errorf("count = %v, want 2", resultMap["count"])
	}
	results, ok := resultMap["results"].([]map[string]any)
	if !ok {
		t.Fatal("results should be a slice of maps")
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0]["doc_id"] != "d1" {
		t.Errorf("results[0].doc_id = %v, want %q", results[0]["doc_id"], "d1")
	}
	if results[0]["score"] != 0.95 {
		t.Errorf("results[0].score = %v, want 0.95", results[0]["score"])
	}
}

func TestKnowledgeSearch_Execute_MissingQuery(t *testing.T) {
	ks := NewKnowledgeSearch(nil)
	ctx := skilldomain.NewSkillContext(context.Background(), "s1", "u1", "t1", "tr1", "user")

	// No query parameter
	_, err := ks.Execute(ctx, map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing query")
	}

	// Empty query string
	_, err = ks.Execute(ctx, map[string]any{"query": ""})
	if err == nil {
		t.Fatal("expected error for empty query")
	}

	// Wrong type for query
	_, err = ks.Execute(ctx, map[string]any{"query": 123})
	if err == nil {
		t.Fatal("expected error for non-string query")
	}
}

func TestKnowledgeSearch_Execute_CustomTopK_Float64(t *testing.T) {
	mockResults := []knowledge.SearchResult{
		{ChunkID: "c1", DocID: "d1", DocTitle: "Doc", Content: "Content", Score: 0.9, Source: "fulltext"},
	}

	patches := gomonkey.ApplyMethodFunc(&knowledgepkg.Service{}, "Search",
		func(userID, query string, topK int, role string) ([]knowledge.SearchResult, error) {
			if topK != 10 {
				t.Errorf("topK = %d, want 10 (parsed from float64)", topK)
			}
			return mockResults, nil
		})
	defer patches.Reset()

	ks := NewKnowledgeSearch(&knowledgepkg.Service{})
	ctx := skilldomain.NewSkillContext(context.Background(), "s1", "u1", "t1", "tr1", "user")

	result, err := ks.Execute(ctx, map[string]any{
		"query": "test",
		"top_k": float64(10),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestKnowledgeSearch_Execute_CustomTopK_Int(t *testing.T) {
	mockResults := []knowledge.SearchResult{
		{ChunkID: "c1", DocID: "d1", DocTitle: "Doc", Content: "Content", Score: 0.9, Source: "fulltext"},
	}

	patches := gomonkey.ApplyMethodFunc(&knowledgepkg.Service{}, "Search",
		func(userID, query string, topK int, role string) ([]knowledge.SearchResult, error) {
			if topK != 20 {
				t.Errorf("topK = %d, want 20", topK)
			}
			return mockResults, nil
		})
	defer patches.Reset()

	ks := NewKnowledgeSearch(&knowledgepkg.Service{})
	ctx := skilldomain.NewSkillContext(context.Background(), "s1", "u1", "t1", "tr1", "user")

	result, err := ks.Execute(ctx, map[string]any{
		"query": "test",
		"top_k": 20,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestKnowledgeSearch_Execute_TopKBelowMinimum(t *testing.T) {
	patches := gomonkey.ApplyMethodFunc(&knowledgepkg.Service{}, "Search",
		func(userID, query string, topK int, role string) ([]knowledge.SearchResult, error) {
			if topK != 1 {
				t.Errorf("topK = %d, want 1 (clamped from 0)", topK)
			}
			return nil, nil
		})
	defer patches.Reset()

	ks := NewKnowledgeSearch(&knowledgepkg.Service{})
	ctx := skilldomain.NewSkillContext(context.Background(), "s1", "u1", "t1", "tr1", "user")

	_, err := ks.Execute(ctx, map[string]any{
		"query": "test",
		"top_k": 0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKnowledgeSearch_Execute_TopKAboveMaximum(t *testing.T) {
	patches := gomonkey.ApplyMethodFunc(&knowledgepkg.Service{}, "Search",
		func(userID, query string, topK int, role string) ([]knowledge.SearchResult, error) {
			if topK != 50 {
				t.Errorf("topK = %d, want 50 (clamped from 100)", topK)
			}
			return nil, nil
		})
	defer patches.Reset()

	ks := NewKnowledgeSearch(&knowledgepkg.Service{})
	ctx := skilldomain.NewSkillContext(context.Background(), "s1", "u1", "t1", "tr1", "user")

	_, err := ks.Execute(ctx, map[string]any{
		"query": "test",
		"top_k": 100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKnowledgeSearch_Execute_RoleFromContext(t *testing.T) {
	patches := gomonkey.ApplyMethodFunc(&knowledgepkg.Service{}, "Search",
		func(userID, query string, topK int, role string) ([]knowledge.SearchResult, error) {
			if role != "admin_role" {
				t.Errorf("role = %q, want %q (from context)", role, "admin_role")
			}
			return nil, nil
		})
	defer patches.Reset()

	ks := NewKnowledgeSearch(&knowledgepkg.Service{})
	ctx := skilldomain.NewSkillContext(context.Background(), "s1", "u1", "t1", "tr1", "admin_role")

	_, err := ks.Execute(ctx, map[string]any{
		"query": "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKnowledgeSearch_Execute_RoleOverrideFromParams(t *testing.T) {
	patches := gomonkey.ApplyMethodFunc(&knowledgepkg.Service{}, "Search",
		func(userID, query string, topK int, role string) ([]knowledge.SearchResult, error) {
			if role != "override_role" {
				t.Errorf("role = %q, want %q (overridden from params)", role, "override_role")
			}
			return nil, nil
		})
	defer patches.Reset()

	ks := NewKnowledgeSearch(&knowledgepkg.Service{})
	ctx := skilldomain.NewSkillContext(context.Background(), "s1", "u1", "t1", "tr1", "default_role")

	_, err := ks.Execute(ctx, map[string]any{
		"query": "test",
		"role":  "override_role",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKnowledgeSearch_Execute_EmptyRoleParamUsesContext(t *testing.T) {
	patches := gomonkey.ApplyMethodFunc(&knowledgepkg.Service{}, "Search",
		func(userID, query string, topK int, role string) ([]knowledge.SearchResult, error) {
			if role != "ctx_role" {
				t.Errorf("role = %q, want %q (from context when param is empty)", role, "ctx_role")
			}
			return nil, nil
		})
	defer patches.Reset()

	ks := NewKnowledgeSearch(&knowledgepkg.Service{})
	ctx := skilldomain.NewSkillContext(context.Background(), "s1", "u1", "t1", "tr1", "ctx_role")

	_, err := ks.Execute(ctx, map[string]any{
		"query": "test",
		"role":  "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKnowledgeSearch_Execute_SearchError(t *testing.T) {
	searchErr := errors.New("search service unavailable")
	patches := gomonkey.ApplyMethodFunc(&knowledgepkg.Service{}, "Search",
		func(userID, query string, topK int, role string) ([]knowledge.SearchResult, error) {
			return nil, searchErr
		})
	defer patches.Reset()

	ks := NewKnowledgeSearch(&knowledgepkg.Service{})
	ctx := skilldomain.NewSkillContext(context.Background(), "s1", "u1", "t1", "tr1", "user")

	_, err := ks.Execute(ctx, map[string]any{
		"query": "test",
	})
	if err == nil {
		t.Fatal("expected error from Search, got nil")
	}
}

func TestKnowledgeSearch_Execute_EmptyResults(t *testing.T) {
	patches := gomonkey.ApplyMethodFunc(&knowledgepkg.Service{}, "Search",
		func(userID, query string, topK int, role string) ([]knowledge.SearchResult, error) {
			return nil, nil
		})
	defer patches.Reset()

	ks := NewKnowledgeSearch(&knowledgepkg.Service{})
	ctx := skilldomain.NewSkillContext(context.Background(), "s1", "u1", "t1", "tr1", "user")

	result, err := ks.Execute(ctx, map[string]any{
		"query": "nonexistent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}
	if count, ok := resultMap["count"].(int); !ok || count != 0 {
		t.Errorf("count = %v, want 0", resultMap["count"])
	}
}

func TestKnowledgeSearch_Execute_TopK_NegativeValue(t *testing.T) {
	patches := gomonkey.ApplyMethodFunc(&knowledgepkg.Service{}, "Search",
		func(userID, query string, topK int, role string) ([]knowledge.SearchResult, error) {
			if topK != 1 {
				t.Errorf("topK = %d, want 1 (clamped from -5)", topK)
			}
			return nil, nil
		})
	defer patches.Reset()

	ks := NewKnowledgeSearch(&knowledgepkg.Service{})
	ctx := skilldomain.NewSkillContext(context.Background(), "s1", "u1", "t1", "tr1", "user")

	_, err := ks.Execute(ctx, map[string]any{
		"query": "test",
		"top_k": -5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ========== truncateContent tests ==========

func TestTruncateContent_ShortContent(t *testing.T) {
	result := truncateContent("Hello", 500)
	if result != "Hello" {
		t.Errorf("truncateContent = %q, want %q", result, "Hello")
	}
}

func TestTruncateContent_ExactLength(t *testing.T) {
	result := truncateContent("Hello", 5)
	if result != "Hello" {
		t.Errorf("truncateContent = %q, want %q", result, "Hello")
	}
}

func TestTruncateContent_OverLength(t *testing.T) {
	longContent := ""
	for i := 0; i < 600; i++ {
		longContent += "a"
	}
	result := truncateContent(longContent, 500)
	expected := longContent[:500] + "..."
	if result != expected {
		t.Errorf("truncateContent length = %d, want %d", len(result), len(expected))
	}
	if len(result) != 503 {
		t.Errorf("truncated length = %d, want 503 (500 + '...')", len(result))
	}
}

func TestTruncateContent_EmptyContent(t *testing.T) {
	result := truncateContent("", 100)
	if result != "" {
		t.Errorf("truncateContent = %q, want %q", result, "")
	}
}

func TestTruncateContent_TrimsWhitespaceWhenTruncated(t *testing.T) {
	// Truncate a long string that has whitespace at the truncation boundary
	content := "  "
	for i := 0; i < 600; i++ {
		content += "a"
	}
	result := truncateContent(content, 500)
	// The first 500 chars are "  " + 498 "a"s, then trimmed to remove leading spaces
	if len(result) != 501 {
		t.Errorf("truncated length = %d, want 501 (500 trimmed chars + '...')", len(result))
	}
	if result[0] == ' ' {
		t.Error("result should not have leading whitespace after trim")
	}
}

func TestTruncateContent_ShortContentWithSpaces(t *testing.T) {
	// Short content is returned as-is without trimming
	result := truncateContent("  hello world  ", 500)
	if result != "  hello world  " {
		t.Errorf("truncateContent = %q, want %q (not trimmed for short content)", result, "  hello world  ")
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
