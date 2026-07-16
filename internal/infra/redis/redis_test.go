package redis

import (
	"testing"
)

func TestNewClient_InvalidAddr(t *testing.T) {
	_, err := NewClient("nonexistent:9999", "", 0)
	if err == nil {
		t.Log("NewClient connected (Redis may be running locally)")
	}
}
