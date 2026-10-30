package task

import (
	"strings"
	"testing"
)

func TestValidateTaskImages_TotalTooLarge(t *testing.T) {
	// Five ~1.2MiB images: each is under the per-image cap (2MiB), but the
	// combined decoded total exceeds MaxTaskTotalBytes (5MiB).
	mid := strings.Repeat("A", 1200*1024*4/3+4) // base64 of ~1.2MiB
	images := make([]ImagePart, 5)
	for i := range images {
		images[i] = ImagePart{Data: mid, MimeType: "image/png"}
	}

	_, err := ValidateTaskImages(images)
	if err != ErrTaskImageTooLarge {
		t.Fatalf("got %v, want ErrTaskImageTooLarge (total exceeded)", err)
	}
}

func TestEncodeTaskImages_Empty(t *testing.T) {
	got, err := EncodeTaskImages(nil)
	if err != nil {
		t.Fatalf("encode empty: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestDecodeTaskImages_BadJSON(t *testing.T) {
	if _, err := DecodeTaskImages("not-json"); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}
