package artifact

import (
	"testing"
)

func TestNewStorage_TypeExists(t *testing.T) {
	// Just verify the type compiles and exists
	var s *Storage
	if s != nil {
		t.Error("nil Storage should be nil")
	}
	_ = s
}
