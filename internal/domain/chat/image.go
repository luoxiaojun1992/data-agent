package chat

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// Image attachment limits: at most 5 images per message; each image at most
// 2 MiB decoded; at most 5 MiB decoded in total per message. The caps keep
// persisted documents (chat session events, task params) well below the
// 16 MiB BSON document limit — chat events are stored twice (events +
// raw_events) so the total cap must stay conservative.
const (
	MaxImages     = 5
	MaxImageBytes = 2 * 1024 * 1024
	MaxTotalBytes = 5 * 1024 * 1024
)

// AllowedImageMimes is the whitelist of accepted image MIME types.
var AllowedImageMimes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
	"image/gif":  true,
}

// ValidateImages enforces the attachment limits and decodes each image's
// base64 payload, returning the decoded byte slices in order. Errors returned
// are the shared domain errors (ErrTooManyImages / ErrImageTooLarge /
// ErrInvalidImage) so both chat and task paths map them to 400 identically.
func ValidateImages(images []ImagePart) ([][]byte, error) {
	if len(images) > MaxImages {
		return nil, ErrTooManyImages
	}
	decoded := make([][]byte, 0, len(images))
	var total int
	for _, img := range images {
		if !AllowedImageMimes[strings.ToLower(strings.TrimSpace(img.MimeType))] {
			return nil, ErrInvalidImage
		}
		data, derr := base64.StdEncoding.DecodeString(img.Data)
		if derr != nil {
			return nil, ErrInvalidImage
		}
		if len(data) > MaxImageBytes {
			return nil, ErrImageTooLarge
		}
		total += len(data)
		if total > MaxTotalBytes {
			return nil, ErrImageTooLarge
		}
		decoded = append(decoded, data)
	}
	return decoded, nil
}

// EncodeImages serializes image attachments to a JSON string. Task params
// persist as map[string]interface{} and round-trip through MongoDB, where a
// nested struct slice degrades into primitive.D/primitive.A; a JSON string
// keeps the boundary lossless and simple to recover in the worker.
func EncodeImages(images []ImagePart) (string, error) {
	if len(images) == 0 {
		return "", nil
	}
	b, err := json.Marshal(images)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DecodeImages parses the JSON produced by EncodeImages.
func DecodeImages(s string) ([]ImagePart, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var images []ImagePart
	if err := json.Unmarshal([]byte(s), &images); err != nil {
		return nil, err
	}
	return images, nil
}
