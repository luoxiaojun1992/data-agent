package queue

import (
	"testing"
)

func TestStreamType(t *testing.T) {
	var s *Stream
	if s != nil {
		t.Error("nil Stream should be nil")
	}
}
