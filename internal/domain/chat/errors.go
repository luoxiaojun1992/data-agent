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
)
