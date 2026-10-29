package feishu

import (
	"context"
	"testing"

	"github.com/agiledragon/gomonkey/v2"

	"github.com/luoxiaojun1992/data-agent/internal/domain/feishu"
	"github.com/luoxiaojun1992/data-agent/internal/infra/vault"
)

// &vault.Client{} carries a nil inner hashicorp client, so Store/Retrieve fail
// deterministically — exercising the vault-error branches (distinct from the
// nil-vault branches already covered in config_service_test.go).

func TestStoreSecret_VaultStoreError(t *testing.T) {
	s := NewConfigService(&fakeFeishuRepo{}, newSessionStub(t), &vault.Client{})
	cfg := &feishu.Config{ID: "f1"}
	if err := s.storeSecret(context.Background(), cfg, "secret"); err == nil {
		t.Fatal("expected error when vault.Store fails")
	}
	if cfg.VaultSecretPath != "" {
		t.Errorf("VaultSecretPath should stay empty on store failure, got %q", cfg.VaultSecretPath)
	}
}

func TestResolveSecret_VaultRetrieveError(t *testing.T) {
	s := NewConfigService(&fakeFeishuRepo{}, newSessionStub(t), &vault.Client{})
	cfg := &feishu.Config{VaultSecretPath: "data-agent/feishu/f1/app_secret", AppSecret: "leak"}
	s.resolveSecret(context.Background(), cfg)
	if cfg.AppSecret != "" {
		t.Errorf("AppSecret should be cleared when vault retrieve fails, got %q", cfg.AppSecret)
	}
}

func TestStoreSecret_Success(t *testing.T) {
	vc := &vault.Client{}
	var storedPath, storedVal string
	patches := gomonkey.ApplyMethodFunc(vc, "Store",
		func(_ context.Context, path, value string) error {
			storedPath, storedVal = path, value
			return nil
		})
	t.Cleanup(patches.Reset)

	s := NewConfigService(&fakeFeishuRepo{}, newSessionStub(t), vc)
	cfg := &feishu.Config{ID: "f1"}
	if err := s.storeSecret(context.Background(), cfg, "secret"); err != nil {
		t.Fatalf("storeSecret() error = %v", err)
	}
	if storedPath != "data-agent/feishu/f1/app_secret" {
		t.Errorf("stored path = %q, want data-agent/feishu/f1/app_secret", storedPath)
	}
	if storedVal != "secret" {
		t.Errorf("stored value = %q, want secret", storedVal)
	}
	if cfg.VaultSecretPath != storedPath {
		t.Errorf("VaultSecretPath = %q, want %q", cfg.VaultSecretPath, storedPath)
	}
	if cfg.AppSecret != "secret" {
		t.Errorf("AppSecret = %q, want secret (in-memory for API response)", cfg.AppSecret)
	}
}

func TestResolveSecret_Success(t *testing.T) {
	vc := &vault.Client{}
	patches := gomonkey.ApplyMethodFunc(vc, "Retrieve",
		func(_ context.Context, path string) (string, error) {
			return "plain", nil
		})
	t.Cleanup(patches.Reset)

	s := NewConfigService(&fakeFeishuRepo{}, newSessionStub(t), vc)
	cfg := &feishu.Config{VaultSecretPath: "p", AppSecret: ""}
	s.resolveSecret(context.Background(), cfg)
	if cfg.AppSecret != "plain" {
		t.Errorf("AppSecret = %q, want plain", cfg.AppSecret)
	}
}
