package chat

import (
	"encoding/json"
	"testing"
)

// EncodeImages / DecodeImages are the JSON round-trip boundary for image
// attachments persisted in task params. The error branches (json.Marshal of a
// string-field-only struct cannot fail; json.Unmarshal of malformed input) are
// exercised here for DecodeImages; EncodeImages's Marshal error branch is
// unreachable for ImagePart (no dynamic types) and therefore untestable.

func TestEncodeImages_Empty(t *testing.T) {
	got, err := EncodeImages(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("EncodeImages(nil) = %q, want empty", got)
	}
}

func TestEncodeImages_Roundtrip(t *testing.T) {
	in := []ImagePart{
		{Data: "aGVsbG8=", MimeType: "image/png"},
		{Data: "aGVsbG8=", MimeType: "image/jpeg"},
	}
	s, err := EncodeImages(in)
	if err != nil {
		t.Fatalf("EncodeImages: %v", err)
	}
	// must be valid JSON array
	var raw []map[string]string
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		t.Fatalf("EncodeImages output is not valid JSON: %v", err)
	}
	if len(raw) != 2 {
		t.Fatalf("encoded %d entries, want 2", len(raw))
	}
}

func TestDecodeImages_Empty(t *testing.T) {
	for _, s := range []string{"", "   ", "\n\t"} {
		got, err := DecodeImages(s)
		if err != nil {
			t.Fatalf("DecodeImages(%q) unexpected error: %v", s, err)
		}
		if got != nil {
			t.Errorf("DecodeImages(%q) = %v, want nil", s, got)
		}
	}
}

func TestDecodeImages_InvalidJSON(t *testing.T) {
	if _, err := DecodeImages("{not json"); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestDecodeImages_Roundtrip(t *testing.T) {
	in := []ImagePart{{Data: "aGVsbG8=", MimeType: "image/gif"}}
	s, err := EncodeImages(in)
	if err != nil {
		t.Fatalf("EncodeImages: %v", err)
	}
	got, err := DecodeImages(s)
	if err != nil {
		t.Fatalf("DecodeImages: %v", err)
	}
	if len(got) != 1 || got[0].MimeType != "image/gif" || got[0].Data != "aGVsbG8=" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}
