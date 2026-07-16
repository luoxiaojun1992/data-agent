package seaweedfs

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient("localhost:9333", "localhost:8080")
	if c == nil {
		t.Error("NewClient should not return nil")
	}
}
