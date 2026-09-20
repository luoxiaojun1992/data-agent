package task

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
)

// Task attachment limits (SPEC-096 D2 定稿：task 域自持校验常量与函数，不再
// import domainchat 的常量/函数，贯彻「各 domain 边界独立」的分层铁律）。
// 数值与 chat 域对齐：最多 5 张图、每张解码后 2MiB、总计 5MiB；文本 100KB。
const (
	MaxTaskImages     = 5
	MaxTaskImageBytes = 2 * 1024 * 1024
	MaxTaskTotalBytes = 5 * 1024 * 1024
	MaxTaskTextBytes  = 100 * 1024
)

// Domain-level attachment errors returned by the task validation helpers.
// Handlers map these to HTTP 400; tests assert on the typed error.
var (
	// ErrTaskTooManyImages indicates more than MaxTaskImages images were sent.
	ErrTaskTooManyImages = errors.New("at most 5 images per task")
	// ErrTaskImageTooLarge indicates an image exceeds the per-image or total
	// size limit.
	ErrTaskImageTooLarge = errors.New("image too large")
	// ErrTaskInvalidImage indicates an image failed base64 decoding or has an
	// unsupported MIME type.
	ErrTaskInvalidImage = errors.New("invalid image data or unsupported image type")
	// ErrTaskTextTooLarge indicates the merged task description (user text +
	// PDF parsed text + Excel parsed text, UTF-8 bytes) exceeds MaxTaskTextBytes.
	ErrTaskTextTooLarge = errors.New("task description exceeds 100KB limit")
)

// AllowedTaskImageMimes is the whitelist of accepted image MIME types.
var AllowedTaskImageMimes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
	"image/gif":  true,
}

// ImagePart is a base64-encoded image attachment carried with a task creation
// request. Structurally identical to chat.ImagePart so the persisted JSON
// (params["images"]) round-trips losslessly between the task and chat paths.
type ImagePart struct {
	// Data is the raw base64-encoded image bytes (no data-URL prefix).
	Data string `json:"data"`
	// MimeType is the image MIME type, e.g. "image/png".
	MimeType string `json:"mime_type"`
}

// ValidateTaskImages enforces the task image limits and decodes each image's
// base64 payload, returning the decoded byte slices in order.
func ValidateTaskImages(images []ImagePart) ([][]byte, error) {
	if len(images) > MaxTaskImages {
		return nil, ErrTaskTooManyImages
	}
	decoded := make([][]byte, 0, len(images))
	var total int
	for _, img := range images {
		if !AllowedTaskImageMimes[strings.ToLower(strings.TrimSpace(img.MimeType))] {
			return nil, ErrTaskInvalidImage
		}
		data, derr := base64.StdEncoding.DecodeString(img.Data)
		if derr != nil {
			return nil, ErrTaskInvalidImage
		}
		if len(data) > MaxTaskImageBytes {
			return nil, ErrTaskImageTooLarge
		}
		total += len(data)
		if total > MaxTaskTotalBytes {
			return nil, ErrTaskImageTooLarge
		}
		decoded = append(decoded, data)
	}
	return decoded, nil
}

// EncodeTaskImages serializes image attachments to a JSON string. Task params
// persist as map[string]interface{} and round-trip through MongoDB, where a
// nested struct slice degrades into primitive.D/primitive.A; a JSON string
// keeps the boundary lossless and simple to recover in the worker.
func EncodeTaskImages(images []ImagePart) (string, error) {
	if len(images) == 0 {
		return "", nil
	}
	b, err := json.Marshal(images)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DecodeTaskImages parses the JSON produced by EncodeTaskImages.
func DecodeTaskImages(s string) ([]ImagePart, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var images []ImagePart
	if err := json.Unmarshal([]byte(s), &images); err != nil {
		return nil, err
	}
	return images, nil
}

// ValidateTaskText runs the XSS guard on user-supplied plain-text task fields
// (title/description) for parity with chat prompt + KB title (SPEC-077/081
// §4.4). PDF/Excel parsed text is document content and therefore exempt from
// XSS (SPEC-096) — it is only length-limited by the caller. The XSS rule itself
// remains single-sourced in domain/security.
func ValidateTaskText(s string) error {
	return security.ValidateXSS(s)
}
