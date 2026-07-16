package qdrant

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient("localhost:6334")
	if c == nil {
		t.Error("NewClient should not return nil")
	}
}

func TestNewClient_EmptyAddr(t *testing.T) {
	c := NewClient("")
	if c == nil {
		t.Error("NewClient with empty addr should not return nil")
	}
}
