package chat

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateImages(t *testing.T) {
	small := "aGVsbG8="                         // "hello" (5 bytes)
	big := strings.Repeat("A", MaxImageBytes*2) // decodes to >2MiB
	tests := []struct {
		name    string
		images  []ImagePart
		wantErr error
	}{
		{"too many", []ImagePart{
			{Data: small, MimeType: "image/png"}, {Data: small, MimeType: "image/png"},
			{Data: small, MimeType: "image/png"}, {Data: small, MimeType: "image/png"},
			{Data: small, MimeType: "image/png"}, {Data: small, MimeType: "image/png"},
		}, ErrTooManyImages},
		{"unsupported mime", []ImagePart{{Data: small, MimeType: "image/tiff"}}, ErrInvalidImage},
		{"bad base64", []ImagePart{{Data: "not-base64!!!", MimeType: "image/png"}}, ErrInvalidImage},
		{"single too large", []ImagePart{{Data: big, MimeType: "image/png"}}, ErrImageTooLarge},
		{"total too large", nil, ErrImageTooLarge},
		{"ok", []ImagePart{
			{Data: small, MimeType: "image/png"},
			{Data: "aGVsbG8=", MimeType: "image/jpeg"},
		}, nil},
	}
	// total too large: five 1.2MiB images (each under per-image cap)
	mid := strings.Repeat("A", 1200*1024*4/3+4) // base64 of ~1.2MiB
	for i := 0; i < len(tests); i++ {
		if tests[i].name == "total too large" {
			for j := 0; j < 5; j++ {
				tests[i].images = append(tests[i].images, ImagePart{Data: mid, MimeType: "image/png"})
			}
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoded, err := ValidateImages(tt.images)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("expected ok, got %v", err)
				}
				if len(decoded) != len(tt.images) {
					t.Fatalf("decoded %d images, want %d", len(decoded), len(tt.images))
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}
