package memoryx

import (
	"testing"
)

// TestListRecent_EmptyUserID verifies the IDOR guard: an empty userID must be
// rejected before any MongoDB read (never read the whole collection).
func TestListRecent_EmptyUserID(t *testing.T) {
	// MongoStorage requires a real *mongo.Collection, so build a zero-value
	// storage; ListRecent must short-circuit on the empty userID before it ever
	// touches the collection.
	s := &MongoStorage{}
	_, _, err := s.ListRecent(t.Context(), "", 5, 0)
	if err == nil {
		t.Fatal("expected error for empty userID")
	}
}
