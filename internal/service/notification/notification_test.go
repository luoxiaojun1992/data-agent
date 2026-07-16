package notification

import (
	"testing"
)

func TestServiceTypeExists(t *testing.T) {
	var s *Service
	if s != nil {
		t.Error("nil Service should be nil")
	}
}
