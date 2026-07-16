package vault

import (
	"os"
	"testing"
)

func TestAPIKeyPath(t *testing.T) {
	if p := APIKeyPath("ns"); p != "ns/model_api_key" {
		t.Errorf("APIKeyPath: got %s", p)
	}
}

func TestHermesAPIKeyPath(t *testing.T) {
	if p := HermesAPIKeyPath("ns"); p != "ns/hermes_api_key" {
		t.Errorf("HermesAPIKeyPath: got %s", p)
	}
}

func TestGetAddr(t *testing.T) {
	addr := GetAddr()
	if addr == "" {
		t.Error("GetAddr should not be empty")
	}
}

func TestMaskValue(t *testing.T) {
	tests := []string{"short", "longsecret12345", "ab", "a"}
	for _, input := range tests {
		masked := MaskValue(input)
		if len(masked) < 1 {
			t.Errorf("MaskValue(%q) returned empty", input)
		}
	}
}

func TestNewClient_NoVault(t *testing.T) {
	os.Setenv("VAULT_ADDR", "http://nonexistent:8200")
	defer os.Unsetenv("VAULT_ADDR")

	c, err := NewClient()
	if err != nil {
		t.Logf("NewClient error (expected): %v", err)
		return
	}
	if c != nil {
		t.Log("NewClient succeeded")
	}
}
