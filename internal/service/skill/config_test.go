package skill

import (
	"context"
	"errors"
	"testing"

	"github.com/luoxiaojun1992/data-agent/internal/domain/skill"
)

type fakeRepo struct {
	searchFn func(ctx context.Context, keyword string, limit int) ([]skill.SkillConfig, error)
}

func (f *fakeRepo) List(ctx context.Context, skip, limit int64) ([]skill.SkillConfig, error) {
	return nil, nil
}
func (f *fakeRepo) Count(ctx context.Context) (int64, error) { return 0, nil }
func (f *fakeRepo) Get(ctx context.Context, name string) (*skill.SkillConfig, error) {
	return nil, nil
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
