package logic

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

type baseURLRepo struct {
	repository.SysConfigRepository
	getFn func(ctx context.Context, key string) (*model.SystemConfig, error)
}

func (f *baseURLRepo) Get(ctx context.Context, key string) (*model.SystemConfig, error) {
	if f.getFn != nil {
		return f.getFn(ctx, key)
	}
	return nil, nil
}

func resetBaseURLGlobals() {
	sysConfigRepo = nil
	baseURLCache = ""
	baseURLCacheAt = time.Time{}
}

func TestResolveInviteBaseURL_NoRepo(t *testing.T) {
	resetBaseURLGlobals()
	if got := ResolveInviteBaseURL(context.Background()); got != "" {
		t.Errorf("ResolveInviteBaseURL() = %q, want empty", got)
	}
}

func TestResolveInviteBaseURL_RepoValue(t *testing.T) {
	resetBaseURLGlobals()
	repo := &baseURLRepo{getFn: func(_ context.Context, key string) (*model.SystemConfig, error) {
		return &model.SystemConfig{Value: "https://override.example.com/"}, nil
	}}
	SetSysConfigRepository(repo)
	defer resetBaseURLGlobals()

	if got := ResolveInviteBaseURL(context.Background()); got != "https://override.example.com" {
		t.Errorf("ResolveInviteBaseURL() = %q, want trimmed override", got)
	}
}

func TestResolveInviteBaseURL_RepoError(t *testing.T) {
	resetBaseURLGlobals()
	repo := &baseURLRepo{getFn: func(_ context.Context, key string) (*model.SystemConfig, error) {
		return nil, errors.New("db down")
	}}
	SetSysConfigRepository(repo)
	defer resetBaseURLGlobals()

	if got := ResolveInviteBaseURL(context.Background()); got != "" {
		t.Errorf("ResolveInviteBaseURL() = %q, want empty on error", got)
	}
}

func TestResolveInviteBaseURL_EmptyValue(t *testing.T) {
	resetBaseURLGlobals()
	repo := &baseURLRepo{getFn: func(_ context.Context, key string) (*model.SystemConfig, error) {
		return &model.SystemConfig{Value: ""}, nil
	}}
	SetSysConfigRepository(repo)
	defer resetBaseURLGlobals()

	if got := ResolveInviteBaseURL(context.Background()); got != "" {
		t.Errorf("ResolveInviteBaseURL() = %q, want empty on empty value", got)
	}
}

func TestSetSysConfigRepository(t *testing.T) {
	resetBaseURLGlobals()
	repo := &baseURLRepo{}
	SetSysConfigRepository(repo)
	if sysConfigRepo != repo {
		t.Error("SetSysConfigRepository did not store the repo")
	}
	resetBaseURLGlobals()
}

func TestResolveInviteBaseURL_CacheHit(t *testing.T) {
	resetBaseURLGlobals()
	baseURLCache = "https://cached.example.com"
	baseURLCacheAt = time.Now()
	defer resetBaseURLGlobals()

	// Cache is fresh (< 30s), so it should be returned without hitting repo.
	if got := resolveInviteBaseURL(); got != "https://cached.example.com" {
		t.Errorf("resolveInviteBaseURL() = %q, want cached value", got)
	}
}

func TestResolveInviteBaseURL_CacheExpired(t *testing.T) {
	resetBaseURLGlobals()
	baseURLCache = "https://stale.example.com"
	baseURLCacheAt = time.Now().Add(-time.Minute) // expired
	repo := &baseURLRepo{getFn: func(_ context.Context, key string) (*model.SystemConfig, error) {
		return &model.SystemConfig{Value: "https://fresh.example.com"}, nil
	}}
	SetSysConfigRepository(repo)
	defer resetBaseURLGlobals()

	// Cache expired → falls through to repo, returns fresh value.
	if got := resolveInviteBaseURL(); got != "https://fresh.example.com" {
		t.Errorf("resolveInviteBaseURL() = %q, want fresh value", got)
	}
}

func TestResolveInviteBaseURL_CacheSetOnFirstHit(t *testing.T) {
	resetBaseURLGlobals()
	repo := &baseURLRepo{getFn: func(_ context.Context, key string) (*model.SystemConfig, error) {
		return &model.SystemConfig{Value: "https://first.example.com"}, nil
	}}
	SetSysConfigRepository(repo)
	defer resetBaseURLGlobals()

	if got := resolveInviteBaseURL(); got != "https://first.example.com" {
		t.Fatalf("resolveInviteBaseURL() = %q", got)
	}
	if baseURLCache != "https://first.example.com" {
		t.Errorf("baseURLCache = %q, want set to resolved value", baseURLCache)
	}
}

func TestGetInviteBaseURL_SystemConfigOverride(t *testing.T) {
	resetBaseURLGlobals()
	repo := &baseURLRepo{getFn: func(_ context.Context, key string) (*model.SystemConfig, error) {
		return &model.SystemConfig{Value: "https://admin.example.com/"}, nil
	}}
	SetSysConfigRepository(repo)
	defer resetBaseURLGlobals()
	t.Setenv("INVITE_BASE_URL", "https://env.example.com")

	// Priority 1: system_config override wins over env.
	if got := GetInviteBaseURL(); got != "https://admin.example.com" {
		t.Errorf("GetInviteBaseURL() = %q, want system_config override", got)
	}
}
