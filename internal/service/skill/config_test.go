package skill

import (
	"context"
	"errors"
	"testing"

	"github.com/luoxiaojun1992/data-agent/internal/domain/skill"
)

type fakeRepo struct {
	searchFn func(ctx context.Context, keyword string, limit int) ([]skill.SkillConfig, error)
	getFn    func(ctx context.Context, name string) (*skill.SkillConfig, error)
}

func (f *fakeRepo) List(ctx context.Context, skip, limit int64) ([]skill.SkillConfig, error) {
	return nil, nil
}
func (f *fakeRepo) Count(ctx context.Context) (int64, error) { return 0, nil }
func (f *fakeRepo) Get(ctx context.Context, name string) (*skill.SkillConfig, error) {
	if f.getFn == nil {
		return nil, nil
	}
	return f.getFn(ctx, name)
}
func (f *fakeRepo) SearchByDescription(ctx context.Context, keyword string, limit int) ([]skill.SkillConfig, error) {
	if f.searchFn == nil {
		return nil, nil
	}
	return f.searchFn(ctx, keyword, limit)
}
func (f *fakeRepo) Upsert(ctx context.Context, cfg skill.SkillConfig) error { return nil }

func TestSearchByDescription_EmptyKeyword(t *testing.T) {
	repo := &fakeRepo{searchFn: func(ctx context.Context, keyword string, limit int) ([]skill.SkillConfig, error) {
		t.Fatal("repo should not be called for empty keyword")
		return nil, nil
	}}
	s := NewConfigService(repo)
	got, err := s.SearchByDescription(context.Background(), "  ", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty keyword should return empty, got %d", len(got))
	}
}

func TestSearchByDescription_DefaultTopN(t *testing.T) {
	var gotLimit int
	repo := &fakeRepo{searchFn: func(ctx context.Context, keyword string, limit int) ([]skill.SkillConfig, error) {
		gotLimit = limit
		return []skill.SkillConfig{{Name: "a"}}, nil
	}}
	s := NewConfigService(repo)
	if _, err := s.SearchByDescription(context.Background(), "file", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotLimit != 5 {
		t.Errorf("default topN: got %d, want 5", gotLimit)
	}
}

func TestSearchByDescription_CapsTopN(t *testing.T) {
	var gotLimit int
	repo := &fakeRepo{searchFn: func(ctx context.Context, keyword string, limit int) ([]skill.SkillConfig, error) {
		gotLimit = limit
		return nil, nil
	}}
	s := NewConfigService(repo)
	if _, err := s.SearchByDescription(context.Background(), "file", 100); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotLimit != 20 {
		t.Errorf("topN cap: got %d, want 20", gotLimit)
	}
}

func TestSearchByDescription_Error(t *testing.T) {
	repo := &fakeRepo{searchFn: func(ctx context.Context, keyword string, limit int) ([]skill.SkillConfig, error) {
		return nil, errors.New("db down")
	}}
	s := NewConfigService(repo)
	if _, err := s.SearchByDescription(context.Background(), "file", 5); err == nil {
		t.Fatal("expected error")
	}
}

func TestSearchByDescription_ReturnsResults(t *testing.T) {
	repo := &fakeRepo{searchFn: func(ctx context.Context, keyword string, limit int) ([]skill.SkillConfig, error) {
		return []skill.SkillConfig{
			{Name: "file_write", DisplayName: "文件写入", Description: "写入文件"},
			{Name: "file_read", DisplayName: "文件查看", Description: "读取文件"},
		}, nil
	}}
	s := NewConfigService(repo)
	got, err := s.SearchByDescription(context.Background(), "文件", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	if got[0].Name != "file_write" {
		t.Errorf("result[0].Name = %q", got[0].Name)
	}
}

func TestRequiresApproval_True(t *testing.T) {
	repo := &fakeRepo{getFn: func(ctx context.Context, name string) (*skill.SkillConfig, error) {
		return &skill.SkillConfig{Name: name, RequiresApproval: true}, nil
	}}
	s := NewConfigService(repo)
	got, err := s.RequiresApproval(context.Background(), "file_delete")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("RequiresApproval should be true")
	}
}

func TestRequiresApproval_False(t *testing.T) {
	repo := &fakeRepo{getFn: func(ctx context.Context, name string) (*skill.SkillConfig, error) {
		return &skill.SkillConfig{Name: name, RequiresApproval: false}, nil
	}}
	s := NewConfigService(repo)
	got, err := s.RequiresApproval(context.Background(), "file_read")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("RequiresApproval should be false")
	}
}

func TestRequiresApproval_UnknownSkill(t *testing.T) {
	// Get returns nil (no such doc) → ConfigService.Get surfaces "unknown skill"
	// which must propagate as an error (fail-closed upstream).
	repo := &fakeRepo{} // getFn nil → returns nil, nil
	s := NewConfigService(repo)
	if _, err := s.RequiresApproval(context.Background(), "nope"); err == nil {
		t.Fatal("expected error for unknown skill")
	}
}

func TestRequiresApproval_DBError(t *testing.T) {
	repo := &fakeRepo{getFn: func(ctx context.Context, name string) (*skill.SkillConfig, error) {
		return nil, errors.New("db down")
	}}
	s := NewConfigService(repo)
	if _, err := s.RequiresApproval(context.Background(), "file_delete"); err == nil {
		t.Fatal("expected error on DB failure")
	}
}

func TestPredefinedSkills_ApprovalFlags(t *testing.T) {
	skills := predefinedSkills()
	// Only file_delete / dir_delete require approval; everything else is false.
	approvalSet := map[string]bool{}
	seen := map[string]bool{}
	for _, sk := range skills {
		if seen[sk.Name] {
			t.Fatalf("duplicate predefined skill: %s", sk.Name)
		}
		seen[sk.Name] = true
		approvalSet[sk.Name] = sk.RequiresApproval
	}
	if !approvalSet["file_delete"] {
		t.Error("file_delete should require approval")
	}
	if !approvalSet["dir_delete"] {
		t.Error("dir_delete should require approval")
	}
	for name, need := range approvalSet {
		if name == "file_delete" || name == "dir_delete" {
			continue
		}
		if need {
			t.Errorf("skill %q should NOT require approval, got true", name)
		}
	}
}
