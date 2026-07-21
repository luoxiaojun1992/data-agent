package mongo

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestNewDomainID(t *testing.T) {
	id := NewDomainID()
	if id == "" {
		t.Fatal("NewDomainID should not return empty string")
	}
	if len(id) != 24 {
		t.Errorf("NewDomainID length: got %d, want 24 (ObjectID hex)", len(id))
	}
	// Must be a valid ObjectID hex.
	if _, err := primitive.ObjectIDFromHex(id); err != nil {
		t.Errorf("NewDomainID %q is not a valid ObjectID hex: %v", id, err)
	}
	// Two calls should produce different IDs.
	id2 := NewDomainID()
	if id == id2 {
		t.Error("NewDomainID should produce unique IDs")
	}
}

func TestObjectIDFromDomainID(t *testing.T) {
	t.Run("valid hex", func(t *testing.T) {
		orig := primitive.NewObjectID()
		got, err := ObjectIDFromDomainID(orig.Hex())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != orig {
			t.Errorf("ObjectIDFromDomainID roundtrip: got %v, want %v", got, orig)
		}
	})

	t.Run("empty id returns error", func(t *testing.T) {
		_, err := ObjectIDFromDomainID("")
		if err == nil {
			t.Fatal("expected error for empty id")
		}
	})

	t.Run("invalid hex returns error", func(t *testing.T) {
		_, err := ObjectIDFromDomainID("not-a-hex")
		if err == nil {
			t.Fatal("expected error for invalid hex")
		}
	})
}

func TestDomainIDFromObjectID(t *testing.T) {
	orig := primitive.NewObjectID()
	got := DomainIDFromObjectID(orig)
	if got != orig.Hex() {
		t.Errorf("DomainIDFromObjectID: got %q, want %q", got, orig.Hex())
	}
	// Nil ObjectID should produce the zero-hex representation, not panic.
	nilHex := DomainIDFromObjectID(primitive.NilObjectID)
	if nilHex != "000000000000000000000000" {
		t.Errorf("DomainIDFromObjectID(NilObjectID): got %q, want zero hex", nilHex)
	}
}
