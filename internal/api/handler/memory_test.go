package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ieshan/adk-go-memory/adapter"
	"github.com/ieshan/idx"

	"github.com/luoxiaojun1992/data-agent/internal/adk/memoryx"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

func TestNewMemoryHandler(t *testing.T) {
	h := NewMemoryHandler(nil, nil, "app", nil, nil)
	if h == nil {
		t.Fatal("handler should not be nil")
	}
	if h.appName != "app" {
		t.Errorf("appName = %q", h.appName)
	}
}

// fakeStorage is a minimal memoryx.Storage double; only List is customizable.
type fakeStorage struct {
	listFn func(ctx context.Context, userID, query string, page, pageSize int) ([]adapter.Observation, int64, error)
}

func (f *fakeStorage) Store(context.Context, *adapter.Observation) error { return nil }
func (f *fakeStorage) GetByID(context.Context, idx.ID) (*adapter.Observation, error) {
	return nil, nil
}
func (f *fakeStorage) Search(context.Context, *adapter.SearchOptions) ([]adapter.SearchResult, error) {
	return nil, nil
}
func (f *fakeStorage) List(ctx context.Context, userID, query string, page, pageSize int) ([]adapter.Observation, int64, error) {
	if f.listFn != nil {
		return f.listFn(ctx, userID, query, page, pageSize)
	}
	return nil, 0, nil
}
func (f *fakeStorage) ListRecent(context.Context, string, int, int) ([]adapter.Observation, int64, error) {
	return nil, 0, nil
}

var _ memoryx.Storage = (*fakeStorage)(nil)

// fakeMemoryUserRepo embeds the interface and overrides only FindByID.
type fakeMemoryUserRepo struct {
	repository.UserRepository
	findByIDFn func(ctx context.Context, id string) (*model.User, error)
}

func (f *fakeMemoryUserRepo) FindByID(ctx context.Context, id string) (*model.User, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(ctx, id)
	}
	return nil, nil
}

// fakeMemorySessionRepo embeds the interface and overrides only Get.
type fakeMemorySessionRepo struct {
	repository.SessionRepository
	getFn func(ctx context.Context, id string) (*repository.SessionRecord, error)
}

func (f *fakeMemorySessionRepo) Get(ctx context.Context, id string) (*repository.SessionRecord, error) {
	if f.getFn != nil {
		return f.getFn(ctx, id)
	}
	return nil, nil
}

func newMemoryHandlerForTest(storage memoryx.Storage, userRepo repository.UserRepository, sessionRepo repository.SessionRepository) *MemoryHandler {
	return NewMemoryHandler(nil, storage, "test-app", userRepo, sessionRepo)
}

// ── List handler ─────────────────────────────────────────────────────

func TestMemoryList_NonAdminScopedToSelf(t *testing.T) {
	st := &fakeStorage{}
	var gotTarget string
	st.listFn = func(_ context.Context, userID, query string, page, pageSize int) ([]adapter.Observation, int64, error) {
		gotTarget = userID
		return nil, 0, nil
	}
	h := newMemoryHandlerForTest(st, nil, nil)

	c, w := newGinContext("GET", "/memory/list?user_id=other&page=1&page_size=20", "")
	c.Set("user_id", "u1")
	c.Set("role", "user")
	h.List(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotTarget != "u1" {
		t.Errorf("targetUser = %q, want u1 (non-admin forced to self)", gotTarget)
	}
}

func TestMemoryList_SystemAdminCanQueryOthers(t *testing.T) {
	st := &fakeStorage{}
	var gotTarget string
	st.listFn = func(_ context.Context, userID, query string, page, pageSize int) ([]adapter.Observation, int64, error) {
		gotTarget = userID
		return nil, 0, nil
	}
	h := newMemoryHandlerForTest(st, nil, nil)

	c, w := newGinContext("GET", "/memory/list?user_id=other", "")
	c.Set("user_id", "u1")
	c.Set("role", "system_admin")
	h.List(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotTarget != "other" {
		t.Errorf("targetUser = %q, want other (system_admin sees all)", gotTarget)
	}
}

func TestMemoryList_StorageError(t *testing.T) {
	st := &fakeStorage{}
	st.listFn = func(_ context.Context, userID, query string, page, pageSize int) ([]adapter.Observation, int64, error) {
		return nil, 0, errors.New("db down")
	}
	h := newMemoryHandlerForTest(st, nil, nil)

	c, w := newGinContext("GET", "/memory/list", "")
	c.Set("user_id", "u1")
	c.Set("role", "user")
	h.List(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestMemoryList_Success(t *testing.T) {
	st := &fakeStorage{}
	st.listFn = func(_ context.Context, userID, query string, page, pageSize int) ([]adapter.Observation, int64, error) {
		return []adapter.Observation{{ID: idx.ID{1}, Content: "hi"}}, int64(1), nil
	}
	h := newMemoryHandlerForTest(st, nil, nil)

	c, w := newGinContext("GET", "/memory/list", "")
	c.Set("user_id", "u1")
	c.Set("role", "user")
	h.List(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ── enrichWithUsers ──────────────────────────────────────────────────

func TestEnrichWithUsers_NilRepo(t *testing.T) {
	obs := []adapter.Observation{
		{ID: idx.ID{1}, Content: "hello", Level: adapter.LevelExplicit, UserID: "u1", AppName: "app"},
	}
	items := enrichWithUsers(context.Background(), nil, nil, obs)

	if len(items) != 1 {
		t.Fatalf("len = %d, want 1", len(items))
	}
	if items[0]["Content"] != "hello" {
		t.Errorf("Content = %v", items[0]["Content"])
	}
	if _, ok := items[0]["UserEmail"]; ok {
		t.Errorf("nil repo should not add UserEmail, got %v", items[0]["UserEmail"])
	}
}

func TestEnrichWithUsers_WithRepo(t *testing.T) {
	userRepo := &fakeMemoryUserRepo{findByIDFn: func(_ context.Context, id string) (*model.User, error) {
		return &model.User{Username: "alice@example.com"}, nil
	}}
	sessionRepo := &fakeMemorySessionRepo{getFn: func(_ context.Context, id string) (*repository.SessionRecord, error) {
		return &repository.SessionRecord{ID: id, Title: "Q3 复盘"}, nil
	}}

	obs := []adapter.Observation{{ID: idx.ID{2}, Content: "c", UserID: "u1", SessionID: "s1"}}
	items := enrichWithUsers(context.Background(), userRepo, sessionRepo, obs)

	if len(items) != 1 {
		t.Fatalf("len = %d, want 1", len(items))
	}
	if items[0]["UserEmail"] != "alice@example.com" {
		t.Errorf("UserEmail = %v, want alice@example.com", items[0]["UserEmail"])
	}
	if items[0]["SessionTitle"] != "Q3 复盘" {
		t.Errorf("SessionTitle = %v, want Q3 复盘", items[0]["SessionTitle"])
	}
}

func TestEnrichWithUsers_FindByIDErrorFallsBackToUserID(t *testing.T) {
	userRepo := &fakeMemoryUserRepo{findByIDFn: func(_ context.Context, id string) (*model.User, error) {
		return nil, errors.New("nf")
	}}
	obs := []adapter.Observation{{ID: idx.ID{3}, Content: "c", UserID: "u1"}}
	items := enrichWithUsers(context.Background(), userRepo, nil, obs)

	if items[0]["UserEmail"] != "u1" {
		t.Errorf("UserEmail = %v, want fallback u1", items[0]["UserEmail"])
	}
}

var _ = gin.H{}
