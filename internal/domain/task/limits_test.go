package task

import (
	"encoding/base64"
	"testing"
)

func TestValidateTaskImages(t *testing.T) {
	small := base64.StdEncoding.EncodeToString([]byte("hello"))
	big := base64.StdEncoding.EncodeToString(make([]byte, MaxTaskImageBytes+1))
	tooMany := make([]ImagePart, MaxTaskImages+1)
	for i := range tooMany {
		tooMany[i] = ImagePart{Data: small, MimeType: "image/png"}
	}

	tests := []struct {
		name    string
		images  []ImagePart
		wantErr error
	}{
		{"empty", nil, nil},
		{"one ok", []ImagePart{{Data: small, MimeType: "image/png"}}, nil},
		{"too many", tooMany, ErrTaskTooManyImages},
		{"unsupported mime", []ImagePart{{Data: small, MimeType: "image/tiff"}}, ErrTaskInvalidImage},
		{"bad base64", []ImagePart{{Data: "not-base64!!!", MimeType: "image/png"}}, ErrTaskInvalidImage},
		{"single too large", []ImagePart{{Data: big, MimeType: "image/png"}}, ErrTaskImageTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateTaskImages(tt.images)
			if err != tt.wantErr {
				t.Fatalf("got %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestEncodeDecodeTaskImagesRoundTrip(t *testing.T) {
	images := []ImagePart{{Data: "aGVsbG8=", MimeType: "image/png"}}
	encoded, err := EncodeTaskImages(images)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := DecodeTaskImages(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded) != 1 || decoded[0].Data != "aGVsbG8=" || decoded[0].MimeType != "image/png" {
		t.Fatalf("round-trip mismatch: %+v", decoded)
	}
}

func TestDecodeTaskImagesEmpty(t *testing.T) {
	decoded, err := DecodeTaskImages("")
	if err != nil || decoded != nil {
		t.Fatalf("expected nil,nil for empty input, got %+v err=%v", decoded, err)
	}
}
