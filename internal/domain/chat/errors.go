package chat

import "errors"

// Domain-level errors returned by the ChatService contract. Handlers map
// these to HTTP status codes; tests assert on the typed error rather than
// a string. This keeps the service decoupled from gin while preserving
// transport-relevant semantics.
var (
	// ErrMessagesRequired indicates the request carried no messages.
	ErrMessagesRequired = errors.New("messages required")
	// ErrUserMessageRequired indicates no user message was present.
	ErrUserMessageRequired = errors.New("user message required")
	// ErrUnauthorizedSession indicates the session does not belong to the
	// requesting user or does not exist.
	ErrUnauthorizedSession = errors.New("invalid or unauthorized session")
	// ErrSessionCreateFailed indicates the session could not be created.
	ErrSessionCreateFailed = errors.New("failed to create session")
	// ErrADKSessionInitFailed indicates the ADK session could not be initialized.
	ErrADKSessionInitFailed = errors.New("failed to init agent session")
	// ErrTooManyImages indicates more than the allowed number of image
	// attachments were sent in one message (max 5).
	ErrTooManyImages = errors.New("at most 5 images per message")
	// ErrImageTooLarge indicates an image exceeds the per-image or per-message
	// size limit.
	ErrImageTooLarge = errors.New("image too large")
	// ErrInvalidImage indicates an image failed base64 decoding or has an
	// unsupported MIME type.
	ErrInvalidImage = errors.New("invalid image data or unsupported image type")
)
