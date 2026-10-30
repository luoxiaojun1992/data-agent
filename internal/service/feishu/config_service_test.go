package feishu

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"

	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	"github.com/luoxiaojun1992/data-agent/internal/domain/feishu"
	chatmocks "github.com/luoxiaojun1992/data-agent/internal/domain/chat/mocks"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// fakeFeishuRepo is a hand-written FeishuConfigRepository double. Each method
// delegates to an injectable func so individual tests control the exact
// behavior without a MongoDB (or mockery) dependency.
type fakeFeishuRepo struct {
	createFn        func(ctx context.Context, cfg *feishu.Config) error
	getFn           func(ctx context.Context, id string) (*feishu.Config, error)
	updateFn        func(ctx context.Context, cfg *feishu.Config) error
	deleteFn        func(ctx context.Context, id string) error
	listByUserFn    func(ctx context.Context, userID string, skip, limit int64) ([]*feishu.Config, int64, error)
	findBySessionFn func(ctx context.Context, sessionID string) (*feishu.Config, error)
	allEnabledFn    func(ctx context.Context) ([]*feishu.Config, error)
}

func (f *fakeFeishuRepo) Create(ctx context.Context, cfg *feishu.Config) error {
	if f.createFn != nil {
		return f.createFn(ctx, cfg)
	}
	return nil
}
func (f *fakeFeishuRepo) Get(ctx context.Context, id string) (*feishu.Config, error) {
	if f.getFn != nil {
		return f.getFn(ctx, id)
	}
	return nil, nil
}
func (f *fakeFeishuRepo) Update(ctx context.Context, cfg *feishu.Config) error {
	if f.updateFn != nil {
		return f.updateFn(ctx, cfg)
	}
	return nil
}
func (f *fakeFeishuRepo) Delete(ctx context.Context, id string) error {
	if f.deleteFn != nil {
		return f.deleteFn(ctx, id)
	}
	return nil
}
func (f *fakeFeishuRepo) ListByUser(ctx context.Context, userID string, skip, limit int64) ([]*feishu.Config, int64, error) {
	if f.listByUserFn != nil {
		return f.listByUserFn(ctx, userID, skip, limit)
	}
	return nil, 0, nil
}
func (f *fakeFeishuRepo) FindBySession(ctx context.Context, sessionID string) (*feishu.Config, error) {
	if f.findBySessionFn != nil {
		return f.findBySessionFn(ctx, sessionID)
	}
	return nil, nil
}
func (f *fakeFeishuRepo) AllEnabled(ctx context.Context) ([]*feishu.Config, error) {
	if f.allEnabledFn != nil {
		return f.allEnabledFn(ctx)
	}
	return nil, nil
}

var _ repository.FeishuConfigRepository = (*fakeFeishuRepo)(nil)

func newSessionStub(t *testing.T) *chatmocks.SessionService {
	s := chatmocks.NewSessionService(t)
	// Maybe: only Create* tests exercise this call; the rest construct a
	// ConfigService that never touches the session.
	s.On("CreateFeishuSession", mock.Anything, mock.Anything).Return(&domainchat.Session{ID: "sess_1"}, nil).Maybe()
	return s
}

func TestFeishuSecretPath(t *testing.T) {
	if got := feishuSecretPath("abc"); got != "data-agent/feishu/abc/app_secret" {
		t.Errorf("feishuSecretPath = %q", got)
	}
}

func TestStoreSecret_EmptyPlaintext(t *testing.T) {
	s := NewConfigService(&fakeFeishuRepo{}, newSessionStub(t), nil)
	cfg := &feishu.Config{ID: "f1"}
	if err := s.storeSecret(context.Background(), cfg, ""); err != nil {
		t.Fatalf("empty plaintext should not error, got %v", err)
	}
}

func TestStoreSecret_NilVault(t *testing.T) {
	s := NewConfigService(&fakeFeishuRepo{}, newSessionStub(t), nil)
	cfg := &feishu.Config{ID: "f1"}
	if err := s.storeSecret(context.Background(), cfg, "secret"); err == nil {
		t.Fatal("expected error when vault is unavailable")
	}
}

func TestResolveSecret_NilConfig(t *testing.T) {
	s := NewConfigService(&fakeFeishuRepo{}, newSessionStub(t), nil)
	s.resolveSecret(context.Background(), nil) // must not panic
}

func TestResolveSecret_NoPath(t *testing.T) {
	s := NewConfigService(&fakeFeishuRepo{}, newSessionStub(t), nil)
	cfg := &feishu.Config{AppSecret: "keep"}
	s.resolveSecret(context.Background(), cfg)
	if cfg.AppSecret != "keep" {
		t.Errorf("AppSecret should be untouched when VaultSecretPath is empty, got %q", cfg.AppSecret)
	}
}

func TestResolveSecret_NilVaultClearsSecret(t *testing.T) {
	s := NewConfigService(&fakeFeishuRepo{}, newSessionStub(t), nil)
	cfg := &feishu.Config{VaultSecretPath: "data-agent/feishu/f1/app_secret", AppSecret: "leak"}
	s.resolveSecret(context.Background(), cfg)
	if cfg.AppSecret != "" {
		t.Errorf("AppSecret should be cleared when vault is unavailable, got %q", cfg.AppSecret)
	}
}

func TestCreate_Success(t *testing.T) {
	repo := &fakeFeishuRepo{}
	repo.createFn = func(ctx context.Context, cfg *feishu.Config) error {
		if cfg.SessionID != "sess_1" {
			t.Errorf("SessionID = %q, want sess_1", cfg.SessionID)
		}
		if cfg.Enabled != true {
			t.Errorf("Enabled = %v, want true", cfg.Enabled)
		}
		return nil
	}
	s := NewConfigService(repo, newSessionStub(t), nil)
	cfg, err := s.Create(context.Background(), "u1", "bot", "app_1", "", "m1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if cfg.ID == "" || cfg.UserID != "u1" || cfg.AppID != "app_1" || cfg.ModelID != "m1" {
		t.Errorf("Create() returned unexpected cfg: %+v", cfg)
	}
}

func TestCreate_SessionError(t *testing.T) {
	sess := chatmocks.NewSessionService(t)
	sess.On("CreateFeishuSession", mock.Anything, mock.Anything).Return(nil, errors.New("session boom"))
	s := NewConfigService(&fakeFeishuRepo{}, sess, nil)
	if _, err := s.Create(context.Background(), "u1", "bot", "app_1", "", "m1"); err == nil {
		t.Fatal("expected error when session creation fails")
	}
}

func TestCreate_SecretStoreError(t *testing.T) {
	// vault is nil → storeSecret fails for a non-empty secret.
	s := NewConfigService(&fakeFeishuRepo{}, newSessionStub(t), nil)
	if _, err := s.Create(context.Background(), "u1", "bot", "app_1", "secret", "m1"); err == nil {
		t.Fatal("expected error when app_secret cannot be stored")
	}
}

func TestCreate_RepoError(t *testing.T) {
	repo := &fakeFeishuRepo{createFn: func(ctx context.Context, cfg *feishu.Config) error {
		return errors.New("repo boom")
	}}
	s := NewConfigService(repo, newSessionStub(t), nil)
	if _, err := s.Create(context.Background(), "u1", "bot", "app_1", "", "m1"); err == nil {
		t.Fatal("expected error when repo create fails")
	}
}

func TestListByUser_NormalizesPaging(t *testing.T) {
	var gotSkip, gotLimit int64
	repo := &fakeFeishuRepo{listByUserFn: func(ctx context.Context, userID string, skip, limit int64) ([]*feishu.Config, int64, error) {
		gotSkip, gotLimit = skip, limit
		return []*feishu.Config{{ID: "f1"}}, 1, nil
	}}
	s := NewConfigService(repo, newSessionStub(t), nil)
	// page=0 → 1, pageSize=999 → 10
	list, total, err := s.ListByUser(context.Background(), "u1", 0, 999)
	if err != nil {
		t.Fatalf("ListByUser() error = %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Errorf("ListByUser() = %d items, %d total", len(list), total)
	}
	if gotSkip != 0 || gotLimit != 10 {
		t.Errorf("skip=%d limit=%d, want skip=0 limit=10", gotSkip, gotLimit)
	}
}

func TestListByUser_Error(t *testing.T) {
	repo := &fakeFeishuRepo{listByUserFn: func(ctx context.Context, userID string, skip, limit int64) ([]*feishu.Config, int64, error) {
		return nil, 0, errors.New("db boom")
	}}
	s := NewConfigService(repo, newSessionStub(t), nil)
	if _, _, err := s.ListByUser(context.Background(), "u1", 1, 10); err == nil {
		t.Fatal("expected error")
	}
}

func TestGet_Success(t *testing.T) {
	repo := &fakeFeishuRepo{getFn: func(ctx context.Context, id string) (*feishu.Config, error) {
		return &feishu.Config{ID: id, VaultSecretPath: "p", AppSecret: "leak"}, nil
	}}
	s := NewConfigService(repo, newSessionStub(t), nil)
	cfg, err := s.Get(context.Background(), "f1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if cfg.AppSecret != "" {
		t.Errorf("AppSecret should be cleared by resolveSecret with nil vault, got %q", cfg.AppSecret)
	}
}

func TestGet_Error(t *testing.T) {
	repo := &fakeFeishuRepo{getFn: func(ctx context.Context, id string) (*feishu.Config, error) {
		return nil, errors.New("not found")
	}}
	s := NewConfigService(repo, newSessionStub(t), nil)
	if _, err := s.Get(context.Background(), "f1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdate_Success(t *testing.T) {
	enabled := true
	repo := &fakeFeishuRepo{
		getFn: func(ctx context.Context, id string) (*feishu.Config, error) {
			return &feishu.Config{ID: id}, nil
		},
		updateFn: func(ctx context.Context, cfg *feishu.Config) error {
			if cfg.Name != "new-name" || cfg.AppID != "new-app" || cfg.Enabled != true {
				t.Errorf("unexpected updated cfg: %+v", cfg)
			}
			return nil
		},
	}
	s := NewConfigService(repo, newSessionStub(t), nil)
	err := s.Update(context.Background(), "f1", UpdateConfigRequest{Name: "new-name", AppID: "new-app", Enabled: &enabled})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
}

func TestUpdate_GetError(t *testing.T) {
	repo := &fakeFeishuRepo{getFn: func(ctx context.Context, id string) (*feishu.Config, error) {
		return nil, errors.New("not found")
	}}
	s := NewConfigService(repo, newSessionStub(t), nil)
	if err := s.Update(context.Background(), "f1", UpdateConfigRequest{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdate_SecretStoreError(t *testing.T) {
	repo := &fakeFeishuRepo{getFn: func(ctx context.Context, id string) (*feishu.Config, error) {
		return &feishu.Config{ID: id}, nil
	}}
	s := NewConfigService(repo, newSessionStub(t), nil)
	if err := s.Update(context.Background(), "f1", UpdateConfigRequest{AppSecret: "secret"}); err == nil {
		t.Fatal("expected error when app_secret cannot be stored (nil vault)")
	}
}

func TestDelete(t *testing.T) {
	var deleted string
	repo := &fakeFeishuRepo{deleteFn: func(ctx context.Context, id string) error {
		deleted = id
		return nil
	}}
	s := NewConfigService(repo, newSessionStub(t), nil)
	if err := s.Delete(context.Background(), "f1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if deleted != "f1" {
		t.Errorf("deleted id = %q, want f1", deleted)
	}
}

func TestFindBySession_Success(t *testing.T) {
	repo := &fakeFeishuRepo{findBySessionFn: func(ctx context.Context, sessionID string) (*feishu.Config, error) {
		return &feishu.Config{ID: "f1", VaultSecretPath: "p", AppSecret: "leak"}, nil
	}}
	s := NewConfigService(repo, newSessionStub(t), nil)
	cfg, err := s.FindBySession(context.Background(), "sess_1")
	if err != nil {
		t.Fatalf("FindBySession() error = %v", err)
	}
	if cfg.AppSecret != "" {
		t.Errorf("AppSecret should be cleared, got %q", cfg.AppSecret)
	}
}

func TestFindBySession_Error(t *testing.T) {
	repo := &fakeFeishuRepo{findBySessionFn: func(ctx context.Context, sessionID string) (*feishu.Config, error) {
		return nil, errors.New("not found")
	}}
	s := NewConfigService(repo, newSessionStub(t), nil)
	if _, err := s.FindBySession(context.Background(), "sess_1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestAllEnabled_Success(t *testing.T) {
	repo := &fakeFeishuRepo{allEnabledFn: func(ctx context.Context) ([]*feishu.Config, error) {
		return []*feishu.Config{{ID: "f1", VaultSecretPath: "p", AppSecret: "leak"}}, nil
	}}
	s := NewConfigService(repo, newSessionStub(t), nil)
	list, err := s.AllEnabled(context.Background())
	if err != nil {
		t.Fatalf("AllEnabled() error = %v", err)
	}
	if len(list) != 1 || list[0].AppSecret != "" {
		t.Errorf("AllEnabled() returned unexpected list")
	}
}

func TestAllEnabled_Error(t *testing.T) {
	repo := &fakeFeishuRepo{allEnabledFn: func(ctx context.Context) ([]*feishu.Config, error) {
		return nil, errors.New("db boom")
	}}
	s := NewConfigService(repo, newSessionStub(t), nil)
	if _, err := s.AllEnabled(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}
