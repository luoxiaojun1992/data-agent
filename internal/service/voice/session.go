// Package voice implements the server-side speech-to-text integration
// (SPEC-099). It fronts the whisper sidecar with an HTTP client and buffers the
// frontend's 5s audio chunks in main-backend memory until the recording
// finishes, then transcribes the merged PCM in one shot. The transcription
// result is returned verbatim — no PII redaction and no prompt enhancement (D7).
package voice

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors surfaced by the Manager (translated to HTTP codes by the
// handler).
var (
	// ErrNotFound means the request_id is unknown or not owned by the caller.
	ErrNotFound = errors.New("voice session not found")
	// ErrSeqMismatch means a chunk arrived out of order or was duplicated.
	ErrSeqMismatch = errors.New("chunk seq out of order")
	// ErrTooLarge means a chunk or the accumulated audio exceeded its limit.
	ErrTooLarge = errors.New("audio exceeds size limit")
)

// MaxVoiceBytes caps the total accumulated audio per session: 60s × 16 kHz ×
// 2 bytes/sample = 1,920,000 bytes (SPEC-099 §3.5).
const MaxVoiceBytes = 60 * 16000 * 2

// voiceSession buffers the audio chunks for one in-flight recording. Each
// session is owned by a single user (checked on every append/finish to prevent
// IDOR) and is serialized with its own mutex — a single recording is always
// sequential.
type voiceSession struct {
	id         string
	userID     string
	lang       string
	seq        int64 // last received chunk seq (starts at -1)
	audio      []byte
	createdAt  time.Time
	lastActive time.Time

	mu sync.Mutex
}

// Manager holds the in-memory recording sessions keyed by request_id. It is
// safe for concurrent use and runs a background goroutine to evict abandoned
// sessions. Sessions are never persisted — they live only as long as a
// recording is in flight.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*voiceSession
	ttl      time.Duration
	maxTotal int64
	maxChunk int64
}

// ManagerConfig configures the in-memory session manager.
type ManagerConfig struct {
	TTL      time.Duration // idle timeout for abandoned sessions
	MaxTotal int64         // total accumulated audio bytes per session
	MaxChunk int64         // per-chunk audio bytes
}

// NewManager creates a session manager with sane defaults for any zero value.
func NewManager(cfg ManagerConfig) *Manager {
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = time.Minute
	}
	maxTotal := cfg.MaxTotal
	if maxTotal <= 0 {
		maxTotal = MaxVoiceBytes
	}
	maxChunk := cfg.MaxChunk
	if maxChunk <= 0 {
		maxChunk = 5 * 16000 * 2 // 5s chunk at 16kHz 16-bit
	}
	return &Manager{
		sessions: make(map[string]*voiceSession),
		ttl:      ttl,
		maxTotal: maxTotal,
		maxChunk: maxChunk,
	}
}

// Start registers a new recording session and returns its request_id.
func (m *Manager) Start(userID, lang string) string {
	if lang == "" {
		lang = "auto"
	}
	now := time.Now()
	s := &voiceSession{
		id:         uuid.NewString(),
		userID:     userID,
		lang:       lang,
		seq:        -1,
		createdAt:  now,
		lastActive: now,
	}
	m.mu.Lock()
	m.sessions[s.id] = s
	m.mu.Unlock()
	return s.id
}

// Append validates and buffers a chunk. It enforces strict ordering
// (seq == last+1) and the total/per-chunk byte caps.
func (m *Manager) Append(requestID, userID string, seq int64, chunk []byte) (int, error) {
	m.mu.RLock()
	s := m.sessions[requestID]
	m.mu.RUnlock()
	if s == nil || s.userID != userID {
		return 0, ErrNotFound
	}
	if int64(len(chunk)) > m.maxChunk {
		return 0, ErrTooLarge
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if seq != s.seq+1 {
		return 0, ErrSeqMismatch
	}
	if int64(len(s.audio))+int64(len(chunk)) > m.maxTotal {
		return 0, ErrTooLarge
	}
	s.audio = append(s.audio, chunk...)
	s.seq = seq
	s.lastActive = time.Now()
	return len(s.audio), nil
}

// Finish atomically removes the session and returns its merged audio + language.
func (m *Manager) Finish(requestID, userID string) ([]byte, string, error) {
	m.mu.Lock()
	s := m.sessions[requestID]
	if s == nil || s.userID != userID {
		m.mu.Unlock()
		return nil, "", ErrNotFound
	}
	delete(m.sessions, requestID)
	m.mu.Unlock()

	s.mu.Lock()
	audio := s.audio
	lang := s.lang
	s.mu.Unlock()
	return audio, lang, nil
}

// StartCleanup runs a background goroutine that periodically evicts abandoned
// sessions. Returns an idempotent stop function.
func (m *Manager) StartCleanup() func() {
	ticker := time.NewTicker(30 * time.Second)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				m.evictStale()
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}

func (m *Manager) evictStale() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.sessions {
		s.mu.Lock()
		stale := now.Sub(s.lastActive) > m.ttl
		s.mu.Unlock()
		if stale {
			delete(m.sessions, id)
		}
	}
}
