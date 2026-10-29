package skill

import (
	"context"
	"errors"
	"testing"

	"github.com/luoxiaojun1992/data-agent/internal/domain/skill"
)

// recordingRepo is a richer SkillConfigRepository double than the fakeRepo in
// config_test.go: it records Upsert calls and allows per-test control of every
// method, so SeedSkills / List / Get / Upsert / GetConfig can be exercised.
type recordingRepo struct {
	listFn   func(ctx context.Context, skip, limit int64) ([]skill.SkillConfig, error)
	countFn  func(ctx context.Context) (int64, error)
	getFn    func(ctx context.Context, name string) (*skill.SkillConfig, error)
	upsertFn func(ctx context.Context, cfg skill.SkillConfig) error
	searchFn func(ctx context.Context, keyword string, limit int) ([]skill.SkillConfig, error)

	upserted []string
}

func (r *recordingRepo) List(ctx context.Context, skip, limit int64) ([]skill.SkillConfig, error) {
	if r.listFn != nil {
		return r.listFn(ctx, skip, limit)
	}
	return nil, nil
}
func (r *recordingRepo) Count(ctx context.Context) (int64, error) {
	if r.countFn != nil {
		return r.countFn(ctx)
	}
	return 0, nil
}
func (r *recordingRepo) Get(ctx context.Context, name string) (*skill.SkillConfig, error) {
	if r.getFn != nil {
		return r.getFn(ctx, name)
	}
	return nil, nil
}
func (r *recordingRepo) SearchByDescription(ctx context.Context, keyword string, limit int) ([]skill.SkillConfig, error) {
	if r.searchFn != nil {
		return r.searchFn(ctx, keyword, limit)
	}
	return nil, nil
}
func (r *recordingRepo) Upsert(ctx context.Context, cfg skill.SkillConfig) error {
	r.upserted = append(r.upserted, cfg.Name)
	if r.upsertFn != nil {
		return r.upsertFn(ctx, cfg)
	}
	return nil
}

func contains(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

func TestPredefinedSkills_AllNonEmpty(t *testing.T) {
	pre := predefinedSkills()
	if len(pre) == 0 {
		t.Fatal("predefinedSkills() returned no skills")
	}
	for _, sk := range pre {
		if sk.Name == "" {
			t.Error("found predefined skill with empty Name")
		}
		if sk.DisplayName == "" {
			t.Errorf("skill %q has empty DisplayName", sk.Name)
		}
		if sk.Description == "" {
			t.Errorf("skill %q has empty Description", sk.Name)
		}
	}
}

func TestSeedSkills_UpsertsMissingOnly(t *testing.T) {
	repo := &recordingRepo{
		listFn: func(ctx context.Context, skip, limit int64) ([]skill.SkillConfig, error) {
			return []skill.SkillConfig{{Name: "sql_executor"}}, nil
		},
	}
	s := NewConfigService(repo)
	if err := s.SeedSkills(context.Background()); err != nil {
		t.Fatalf("SeedSkills() error = %v", err)
	}
	if contains(repo.upserted, "sql_executor") {
		t.Error("existing skill sql_executor should not be re-upserted")
	}
	for _, sk := range predefinedSkills() {
		if sk.Name == "sql_executor" {
			continue
		}
		if !contains(repo.upserted, sk.Name) {
			t.Errorf("predefined skill %q was not seeded", sk.Name)
		}
	}
}

func TestSeedSkills_ListError(t *testing.T) {
	repo := &recordingRepo{
		listFn: func(ctx context.Context, skip, limit int64) ([]skill.SkillConfig, error) {
			return nil, errors.New("db down")
		},
	}
	s := NewConfigService(repo)
	if err := s.SeedSkills(context.Background()); err == nil {
		t.Fatal("expected error when list fails")
	}
}

func TestSeedSkills_UpsertErrorTolerated(t *testing.T) {
	// A per-skill Upsert failure is logged, not fatal — seeding continues and
	// returns nil (idempotent, lenient on startup).
	repo := &recordingRepo{
		listFn: func(ctx context.Context, skip, limit int64) ([]skill.SkillConfig, error) {
			return nil, nil
		},
		upsertFn: func(ctx context.Context, cfg skill.SkillConfig) error {
			return errors.New("upsert boom")
		},
	}
	s := NewConfigService(repo)
	if err := s.SeedSkills(context.Background()); err != nil {
		t.Fatalf("SeedSkills() should tolerate upsert failures, got %v", err)
	}
}

func TestList_NormalizesPaging(t *testing.T) {
	var gotSkip, gotLimit int64
	repo := &recordingRepo{
		listFn: func(ctx context.Context, skip, limit int64) ([]skill.SkillConfig, error) {
			gotSkip, gotLimit = skip, limit
			return []skill.SkillConfig{{Name: "a"}}, nil
		},
		countFn: func(ctx context.Context) (int64, error) { return 42, nil },
	}
	s := NewConfigService(repo)
	items, total, err := s.List(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if gotSkip != 0 || gotLimit != 20 {
		t.Errorf("skip=%d limit=%d, want skip=0 limit=20", gotSkip, gotLimit)
	}
	if total != 42 || len(items) != 1 {
		t.Errorf("total=%d items=%d, want total=42 items=1", total, len(items))
	}
}

func TestList_ListError(t *testing.T) {
	repo := &recordingRepo{listFn: func(ctx context.Context, skip, limit int64) ([]skill.SkillConfig, error) {
		return nil, errors.New("db down")
	}}
	s := NewConfigService(repo)
	if _, _, err := s.List(context.Background(), 1, 10); err == nil {
		t.Fatal("expected error")
	}
}

func TestList_CountError(t *testing.T) {
	repo := &recordingRepo{
		listFn:  func(ctx context.Context, skip, limit int64) ([]skill.SkillConfig, error) { return nil, nil },
		countFn: func(ctx context.Context) (int64, error) { return 0, errors.New("count down") },
	}
	s := NewConfigService(repo)
	if _, _, err := s.List(context.Background(), 1, 10); err == nil {
		t.Fatal("expected error when count fails")
	}
}

func TestGet_UnknownSkill(t *testing.T) {
	s := NewConfigService(&recordingRepo{}) // getFn nil → returns nil, nil
	if _, err := s.Get(context.Background(), "nope"); err == nil {
		t.Fatal("expected 'unknown skill' error")
	}
}

func TestGet_Success(t *testing.T) {
	repo := &recordingRepo{getFn: func(ctx context.Context, name string) (*skill.SkillConfig, error) {
		return &skill.SkillConfig{Name: name}, nil
	}}
	s := NewConfigService(repo)
	cfg, err := s.Get(context.Background(), "file_read")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if cfg.Name != "file_read" {
		t.Errorf("Name = %q", cfg.Name)
	}
}

func TestGet_DBError(t *testing.T) {
	repo := &recordingRepo{getFn: func(ctx context.Context, name string) (*skill.SkillConfig, error) {
		return nil, errors.New("db down")
	}}
	s := NewConfigService(repo)
	if _, err := s.Get(context.Background(), "x"); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpsert_InvalidConfig(t *testing.T) {
	s := NewConfigService(&recordingRepo{})
	err := s.Upsert(context.Background(), skill.SkillConfig{Name: "sql_executor", ConfigJSON: `{"max_rows":-1}`})
	if err == nil {
		t.Fatal("expected validation error for negative max_rows")
	}
}

func TestUpsert_Valid(t *testing.T) {
	repo := &recordingRepo{}
	s := NewConfigService(repo)
	err := s.Upsert(context.Background(), skill.SkillConfig{Name: "sql_executor", ConfigJSON: `{"max_rows":10}`})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	if !contains(repo.upserted, "sql_executor") {
		t.Error("valid skill was not upserted")
	}
}

func TestGetConfig_EmptyConfig(t *testing.T) {
	repo := &recordingRepo{getFn: func(ctx context.Context, name string) (*skill.SkillConfig, error) {
		return &skill.SkillConfig{Name: name, ConfigJSON: ""}, nil
	}}
	s := NewConfigService(repo)
	var target map[string]any
	if err := s.GetConfig(context.Background(), "x", &target); err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}
}

func TestGetConfig_Unmarshals(t *testing.T) {
	repo := &recordingRepo{getFn: func(ctx context.Context, name string) (*skill.SkillConfig, error) {
		return &skill.SkillConfig{Name: name, ConfigJSON: `{"max_rows":5}`}, nil
	}}
	s := NewConfigService(repo)
	var target struct {
		MaxRows int `json:"max_rows"`
	}
	if err := s.GetConfig(context.Background(), "x", &target); err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}
	if target.MaxRows != 5 {
		t.Errorf("MaxRows = %d, want 5", target.MaxRows)
	}
}

func TestIsEnabled(t *testing.T) {
	repo := &recordingRepo{getFn: func(ctx context.Context, name string) (*skill.SkillConfig, error) {
		switch name {
		case "on":
			return &skill.SkillConfig{Name: name, Enabled: true}, nil
		case "off":
			return &skill.SkillConfig{Name: name, Enabled: false}, nil
		default:
			return nil, nil
		}
	}}
	s := NewConfigService(repo)
	if !s.IsEnabled(context.Background(), "on") {
		t.Error("IsEnabled(on) should be true")
	}
	if s.IsEnabled(context.Background(), "off") {
		t.Error("IsEnabled(off) should be false")
	}
	if s.IsEnabled(context.Background(), "unknown") {
		t.Error("IsEnabled(unknown) should be false")
	}
}

func TestValidateConfig(t *testing.T) {
	// empty config is always valid
	for _, c := range []string{"", "{}"} {
		if err := validateConfig("anything", c); err != nil {
			t.Errorf("validateConfig(%q) error = %v, want nil", c, err)
		}
	}
	// sql_executor: invalid JSON, negative bounds
	if err := validateConfig("sql_executor", `{bad`); err == nil {
		t.Error("expected error for invalid JSON")
	}
	if err := validateConfig("sql_executor", `{"max_rows":-1}`); err == nil {
		t.Error("expected error for negative max_rows")
	}
	if err := validateConfig("sql_executor", `{"query_timeout":-1}`); err == nil {
		t.Error("expected error for negative query_timeout")
	}
	if err := validateConfig("sql_executor", `{"max_rows":10,"query_timeout":30000}`); err != nil {
		t.Errorf("valid sql_executor config error = %v", err)
	}
	// generic: only JSON validity is checked
	if err := validateConfig("web_search", `{bad`); err == nil {
		t.Error("expected error for invalid generic JSON")
	}
	if err := validateConfig("web_search", `{"top_n":5}`); err != nil {
		t.Errorf("valid generic config error = %v", err)
	}
}
