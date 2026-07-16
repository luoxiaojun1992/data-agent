package vault

import (
	"testing"
)

func TestAPIKeyPath(t *testing.T) {
	path := APIKeyPath("data-agent")
	if path != "data-agent/model_api_key" {
		t.Errorf("APIKeyPath: got %s", path)
	}
}

func TestHermesAPIKeyPath(t *testing.T) {
	path := HermesAPIKeyPath("data-agent")
	if path != "data-agent/hermes_api_key" {
		t.Errorf("HermesAPIKeyPath: got %s", path)
	}
}

func TestGetAddr(t *testing.T) {
	addr := GetAddr()
	if addr == "" {
		t.Error("GetAddr should not be empty")
	}
}

func TestMaskValue(t *testing.T) {
	// MaskValue keeps first 2 and last 2 chars, replaces middle with dots
	masked := MaskValue("abcdefgh")
	// Should be: "ab••••gh" (8 chars → 2+4+2)
	if len(masked) < 6 {
		t.Errorf("MaskValue length too short: %q", masked)
	}

	short := MaskValue("ab")
	if short != "••" {
		t.Errorf("MaskValue('ab') = %q", short)
	}
}
