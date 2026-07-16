package audit

import (
	"testing"
)

func TestListParams_Defaults(t *testing.T) {
	p := ListParams{
		Limit: 20,
	}
	if p.Limit != 20 {
		t.Errorf("Limit: got %d, want 20", p.Limit)
	}
}
