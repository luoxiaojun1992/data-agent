package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"

	whisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"golang.org/x/sync/semaphore"
)

// Transcriber owns a pool of whisper models with warm-up and dynamic growth.
//
// Concurrency model: the Go bindings expose a single underlying
// whisper_context per model (model.NewContext() is a thin wrapper over the
// same C context), so each model serves one request at a time. We therefore
// keep a pool of model instances:
//
//   - warm-up: minPool instances are loaded eagerly at startup;
//   - dynamic growth: when the idle pool is empty we load a new instance
//     on demand, up to maxPool;
//   - the semaphore caps in-flight transcriptions at maxPool, so total
//     instances never exceed maxPool (bounded memory).
type Transcriber struct {
	modelPath string
	threads   int
	audioCtx  int

	sem *semaphore.Weighted // in-flight cap = maxPool

	mu      sync.Mutex
	idle    []whisper.Model // ready instances
	total   int             // total loaded (idle + in-flight), <= maxPool
	minPool int
	maxPool int
}

// NewTranscriber warms up minPool instances and prepares dynamic growth up to
// maxPool. Loading happens outside any lock (it is slow).
func NewTranscriber(modelPath string, minPool, maxPool, threads, audioCtx int) (*Transcriber, error) {
	if minPool < 1 {
		minPool = 1
	}
	if maxPool < minPool {
		maxPool = minPool
	}
	t := &Transcriber{
		modelPath: modelPath,
		threads:   threads,
		audioCtx:  audioCtx,
		sem:       semaphore.NewWeighted(int64(maxPool)),
		minPool:   minPool,
		maxPool:   maxPool,
	}
	for i := 0; i < minPool; i++ {
		m, err := whisper.New(modelPath)
		if err != nil {
			t.Close()
			return nil, fmt.Errorf("warm-up model #%d: %w", i, err)
		}
		t.idle = append(t.idle, m)
		t.total++
	}
	return t, nil
}

// Close releases every idle model. In-flight callers are responsible for
// returning instances before Close (the HTTP server drains first).
func (t *Transcriber) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, m := range t.idle {
		_ = m.Close()
	}
	t.idle = nil
	t.total = 0
}

// acquire borrows a model: reuse an idle one, else grow the pool (bounded by
// maxPool — guaranteed by the semaphore acquired by the caller).
func (t *Transcriber) acquire() (whisper.Model, error) {
	t.mu.Lock()
	if n := len(t.idle); n > 0 {
		m := t.idle[n-1]
		t.idle = t.idle[:n-1]
		t.mu.Unlock()
		return m, nil
	}
	t.total++
	t.mu.Unlock()

	// Load outside the lock — this is slow (model weight + state alloc).
	m, err := whisper.New(t.modelPath)
	if err != nil {
		t.mu.Lock()
		t.total--
		t.mu.Unlock()
		return nil, err
	}
	return m, nil
}

// release returns a model to the idle pool.
func (t *Transcriber) release(m whisper.Model) {
	t.mu.Lock()
	t.idle = append(t.idle, m)
	t.mu.Unlock()
}

// Transcribe converts mono 16kHz int16 PCM to text. lang may be "auto".
func (t *Transcriber) Transcribe(ctx context.Context, pcmInt16 []byte, lang string) (string, error) {
	if len(pcmInt16) == 0 {
		return "", nil
	}

	// In-flight cap: blocks while maxPool transcriptions are running.
	if err := t.sem.Acquire(ctx, 1); err != nil {
		return "", err
	}
	defer t.sem.Release(1)

	model, err := t.acquire()
	if err != nil {
		return "", err
	}
	defer t.release(model)

	wctx, err := model.NewContext()
	if err != nil {
		return "", err
	}
	if err := wctx.SetLanguage(normLang(lang)); err != nil {
		return "", err
	}
	wctx.SetTranslate(false)
	wctx.SetTokenTimestamps(false)
	if t.audioCtx > 0 {
		wctx.SetAudioCtx(uint(t.audioCtx))
	}
	if t.threads > 0 {
		wctx.SetThreads(uint(t.threads))
	}

	if err := wctx.Process(int16ToFloat32(pcmInt16), nil, nil, nil); err != nil {
		return "", err
	}

	var sb strings.Builder
	for {
		seg, err := wctx.NextSegment()
		if err != nil {
			break // io.EOF
		}
		if text := filterSpecial(seg.Text); text != "" {
			if sb.Len() > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(text)
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

// int16ToFloat32 converts little-endian int16 PCM samples to float32 in [-1,1].
func int16ToFloat32(pcm []byte) []float32 {
	out := make([]float32, len(pcm)/2)
	for i := range out {
		v := int16(binary.LittleEndian.Uint16(pcm[i*2:]))
		out[i] = float32(v) / 32768.0
	}
	return out
}

// normLang maps empty to "auto", passes everything else through.
func normLang(lang string) string {
	if lang == "" {
		return "auto"
	}
	return lang
}

// filterSpecial drops special-token fragments ([MUSIC], <|0.00|>, etc.) so
// noise never leaks into the transcript.
func filterSpecial(text string) string {
	fields := strings.Fields(text)
	keep := make([]string, 0, len(fields))
	for _, f := range fields {
		if strings.HasPrefix(f, "[") || strings.HasPrefix(f, "<") {
			continue
		}
		keep = append(keep, f)
	}
	return strings.Join(keep, " ")
}
