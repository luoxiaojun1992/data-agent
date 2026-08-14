package handler

import (
	"testing"
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
