package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidYAML(t *testing.T) {
	content := `
server:
  port: 3000
jwt:
  secret: test-secret
  expiration: 1h
log:
  level: debug
  format: text
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 3000 {
		t.Errorf("port: got %d, want 3000", cfg.Server.Port)
	}
	if cfg.JWT.Secret != "test-secret" {
		t.Errorf("jwt secret: got %s", cfg.JWT.Secret)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("log level: got %s, want debug", cfg.Log.Level)
	}
	if cfg.Log.Format != "text" {
		t.Errorf("log format: got %s", cfg.Log.Format)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	content := `server: [invalid yaml`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Error("invalid YAML should return error")
	}
}
