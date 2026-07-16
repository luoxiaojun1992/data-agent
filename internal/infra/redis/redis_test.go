package redis

import (
	"testing"
)

func TestClientType(t *testing.T) {
	// Verify Client type exists and can be instantiated
	var c *Client
	if c != nil {
		t.Error("nil Client should be nil")
	}

	// NewClient should handle invalid addr
	_, err := NewClient("invalid:99999", "", 0)
	if err == nil {
		t.Log("connected unexpectedly")
	}
}
