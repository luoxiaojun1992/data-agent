package qdrant

import (
	"testing"
)

func TestNewClient_CreatesInstance(t *testing.T) {
	c := NewClient("localhost:6334")
	if c == nil {
		t.Error("NewClient should not return nil")
	}
}
