package apireview

import (
	"testing"
)

func TestGenShortIDLength(t *testing.T) {
	id := genShortID()
	if id == "" {
		t.Error("genShortID should not return empty")
	}
}
