package skill

import (
	"testing"

	skilldomain "github.com/luoxiaojun1992/data-agent/internal/domain/skill"
)

func TestSaveReport_Name(t *testing.T) {
	s := &SaveReport{}
	if got := s.Name(); got != "save_report" {
		t.Errorf("Name() = %q, want %q", got, "save_report")
	}
}

func TestSaveReport_Description(t *testing.T) {
	s := &SaveReport{}
	desc := s.Description()
	if desc == "" {
		t.Error("Description() should not be empty")
	}
}

func TestSaveReport_Parameters(t *testing.T) {
	s := &SaveReport{}
	params := s.Parameters()
	if len(params) < 2 {
		t.Fatal("Parameters() should return at least 2 parameters")
	}
	if params[0].Name != "title" {
		t.Errorf("first param name = %q, want %q", params[0].Name, "title")
	}
	if !params[0].Required {
		t.Error("'title' parameter should be required")
	}
	if params[1].Name != "content" {
		t.Errorf("second param name = %q, want %q", params[1].Name, "content")
	}
	if !params[1].Required {
		t.Error("'content' parameter should be required")
	}
}

func TestSaveReport_Permissions(t *testing.T) {
	s := &SaveReport{}
	perms := s.Permissions()
	if len(perms) == 0 {
		t.Error("Permissions() should not be empty")
	}
	found := false
	for _, p := range perms {
		if p == "skill:save_report" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Permissions() should contain 'skill:save_report'")
	}
}

func TestSaveReport_RateLimit(t *testing.T) {
	s := &SaveReport{}
	rl := s.RateLimit()
	if rl == nil {
		t.Fatal("RateLimit() should not be nil")
	}
	if rl.MaxRequests != 20 {
		t.Errorf("MaxRequests = %d, want 20", rl.MaxRequests)
	}
	if rl.WindowSec != 60 {
		t.Errorf("WindowSec = %d, want 60", rl.WindowSec)
	}
}

func TestSaveReport_Execute_MissingTitle(t *testing.T) {
	s := &SaveReport{}
	ctx := skilldomain.SkillContext{}
	result, err := s.Execute(ctx, map[string]any{})
	if err == nil {
		t.Error("Execute() should return error for missing 'title'")
	}
	if result != nil {
		t.Error("Execute() should return nil result on error")
	}
}

func TestSaveReport_Execute_EmptyTitle(t *testing.T) {
	s := &SaveReport{}
	ctx := skilldomain.SkillContext{}
	result, err := s.Execute(ctx, map[string]any{"title": ""})
	if err == nil {
		t.Error("Execute() should return error for empty 'title'")
	}
	if result != nil {
		t.Error("Execute() should return nil result on error")
	}
}

func TestSaveReport_Execute_MissingContent(t *testing.T) {
	s := &SaveReport{}
	ctx := skilldomain.SkillContext{}
	result, err := s.Execute(ctx, map[string]any{"title": "Report"})
	if err == nil {
		t.Error("Execute() should return error for missing 'content'")
	}
	if result != nil {
		t.Error("Execute() should return nil result on error")
	}
}

func TestSaveReport_Execute_EmptyContent(t *testing.T) {
	s := &SaveReport{}
	ctx := skilldomain.SkillContext{}
	result, err := s.Execute(ctx, map[string]any{
		"title":   "Report",
		"content": "",
	})
	if err == nil {
		t.Error("Execute() should return error for empty 'content'")
	}
	if result != nil {
		t.Error("Execute() should return nil result on error")
	}
}

func TestSaveReport_Execute_WithValidation(t *testing.T) {
	s := &SaveReport{}
	ctx := skilldomain.SkillContext{UserID: "user1", Role: "user"}
	result, err := s.Execute(ctx, map[string]any{
		"title":    "Test Report",
		"content":  "# Introduction\n\nThis is a test report with some content.",
		"validate": true,
	})
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Execute() should return result")
	}
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}
	if resultMap["title"] != "Test Report" {
		t.Errorf("title = %v, want 'Test Report'", resultMap["title"])
	}
}

func TestSaveReport_Execute_SkipValidation(t *testing.T) {
	s := &SaveReport{}
	ctx := skilldomain.SkillContext{UserID: "user1", Role: "user"}
	result, err := s.Execute(ctx, map[string]any{
		"title":    "Simple Report",
		"content":  "Some content",
		"validate": false,
	})
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}
	if resultMap["status"] != "saved" {
		t.Errorf("status = %v, want 'saved'", resultMap["status"])
	}
}

func TestSaveReport_Execute_DefaultValidation_WithSections(t *testing.T) {
	s := &SaveReport{}
	ctx := skilldomain.SkillContext{UserID: "user1", Role: "user"}
	result, err := s.Execute(ctx, map[string]any{
		"title":   "Full Report",
		"content": "# Introduction\nContent here\n\n# Analysis\nMore content\n\n# Conclusion\nFinal words",
	})
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}
	if _, exists := resultMap["valid"]; !exists {
		t.Error("result should contain 'valid' key when validation is enabled (default)")
	}
	if _, exists := resultMap["detected_sections"]; !exists {
		t.Error("result should contain 'detected_sections' key when validation is enabled")
	}
}

func TestSaveReport_Execute_ValidationFailed(t *testing.T) {
	s := &SaveReport{}
	ctx := skilldomain.SkillContext{UserID: "user1", Role: "user"}
	// Report without required sections
	result, err := s.Execute(ctx, map[string]any{
		"title":   "Incomplete Report",
		"content": "Just some random text without proper sections.",
	})
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}
	valid, _ := resultMap["valid"].(bool)
	if valid {
		t.Error("report without required sections should not be valid")
	}
	if _, exists := resultMap["missing_sections"]; !exists {
		t.Error("result should contain 'missing_sections' for invalid report")
	}
	if _, exists := resultMap["feedback"]; !exists {
		t.Error("result should contain 'feedback' for invalid report")
	}
}
