package voice

import (
	"context"
	"errors"
)

// ErrEmptyAudio means finish was called without any buffered chunks.
var ErrEmptyAudio = errors.New("no audio received")

// Service coordinates chunk buffering (Manager) and transcription (Client).
type Service struct {
	client  *Client
	manager *Manager
}

// NewService creates the voice service.
func NewService(client *Client, manager *Manager) *Service {
	return &Service{client: client, manager: manager}
}

// Start registers a recording session and returns its request_id.
func (s *Service) Start(_ context.Context, userID, lang string) string {
	return s.manager.Start(userID, lang)
}

// Chunk buffers a chunk, validating ownership, ordering, and size limits.
func (s *Service) Chunk(_ context.Context, requestID, userID string, seq int64, audio []byte) (int, error) {
	return s.manager.Append(requestID, userID, seq, audio)
}

// Finish merges the buffered chunks and transcribes them in one shot, then
// returns the raw text (no redaction / no enhancement).
func (s *Service) Finish(ctx context.Context, requestID, userID string) (string, error) {
	audio, lang, err := s.manager.Finish(requestID, userID)
	if err != nil {
		return "", err
	}
	if len(audio) == 0 {
		return "", ErrEmptyAudio
	}
	return s.client.Transcribe(ctx, audio, lang)
}
