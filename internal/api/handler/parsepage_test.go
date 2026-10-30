package handler

import (
	"testing"
)

func TestParsePage(t *testing.T) {
	cases := []struct {
		name             string
		query            string
		wantPage, wantSz int
	}{
		{"defaults", "", 1, 20},
		{"valid", "page=3&page_size=50", 3, 50},
		{"page below one", "page=0&page_size=10", 1, 10},
		{"negative page", "page=-5&page_size=10", 1, 10},
		{"page_size below one", "page=2&page_size=0", 2, 20},
		{"page_size above max", "page=2&page_size=500", 2, 20},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ginCtx, _ := newSessionGin("GET", "/x?"+c.query)
			page, size := parsePage(ginCtx)
			if page != c.wantPage || size != c.wantSz {
				t.Fatalf("parsePage(%q) = (%d,%d), want (%d,%d)", c.query, page, size, c.wantPage, c.wantSz)
			}
		})
	}
}
