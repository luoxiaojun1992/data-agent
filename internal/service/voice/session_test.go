package voice

import (
	"testing"
	"time"
)

func TestManager_StartAppendFinish(t *testing.T) {
	m := NewManager(ManagerConfig{})
	id := m.Start("user1", "zh")
	if id == "" {
		t.Fatal("expected non-empty request_id")
	}

	if n, err := m.Append(id, "user1", 0, []byte("aaaa")); err != nil || n != 4 {
		t.Fatalf("append seq0: n=%d err=%v", n, err)
	}
	if n, err := m.Append(id, "user1", 1, []byte("bbbb")); err != nil || n != 8 {
		t.Fatalf("append seq1: n=%d err=%v", n, err)
	}

	audio, lang, err := m.Finish(id, "user1")
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if lang != "zh" {
		t.Errorf("expected lang zh, got %q", lang)
	}
	if string(audio) != "aaaabbbb" {
		t.Errorf("expected merged audio, got %q", audio)
	}
}

func TestManager_StartDefaultsLangToAuto(t *testing.T) {
	m := NewManager(ManagerConfig{})
	id := m.Start("user1", "")
	if _, err := m.Append(id, "user1", 0, []byte("x")); err != nil {
		t.Fatalf("append: %v", err)
	}
	_, lang, err := m.Finish(id, "user1")
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if lang != "auto" {
		t.Errorf("expected auto, got %q", lang)
	}
}

func TestManager_AppendSeqMismatch(t *testing.T) {
	m := NewManager(ManagerConfig{})
	id := m.Start("user1", "zh")

	if _, err := m.Append(id, "user1", 5, []byte("x")); err != ErrSeqMismatch {
		t.Fatalf("expected ErrSeqMismatch for skipped seq, got %v", err)
	}
	if _, err := m.Append(id, "user1", 0, []byte("x")); err != nil {
		t.Fatalf("append seq0: %v", err)
	}
	// Duplicate seq 0 (already received) → mismatch.
	if _, err := m.Append(id, "user1", 0, []byte("x")); err != ErrSeqMismatch {
		t.Fatalf("expected ErrSeqMismatch for duplicate seq, got %v", err)
	}
}

func TestManager_AppendOwnership(t *testing.T) {
	m := NewManager(ManagerConfig{})
	id := m.Start("user1", "zh")

	if _, err := m.Append(id, "user2", 0, []byte("x")); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for foreign user, got %v", err)
	}
	if _, err := m.Append("missing", "user1", 0, []byte("x")); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for unknown id, got %v", err)
	}
}

func TestManager_AppendTooLargeChunk(t *testing.T) {
	m := NewManager(ManagerConfig{MaxChunk: 4})
	id := m.Start("user1", "zh")

	if _, err := m.Append(id, "user1", 0, make([]byte, 5)); err != ErrTooLarge {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}

func TestManager_AppendTooLargeTotal(t *testing.T) {
	m := NewManager(ManagerConfig{MaxChunk: 100, MaxTotal: 8})
	id := m.Start("user1", "zh")

	if _, err := m.Append(id, "user1", 0, []byte("1234")); err != nil {
		t.Fatalf("append seq0: %v", err)
	}
	if _, err := m.Append(id, "user1", 1, []byte("5678")); err != nil {
		t.Fatalf("append seq1: %v", err)
	}
	// Pushing past MaxTotal=8 → too large.
	if _, err := m.Append(id, "user1", 2, []byte("9")); err != ErrTooLarge {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}

func TestManager_FinishOwnership(t *testing.T) {
	m := NewManager(ManagerConfig{})
	id := m.Start("user1", "zh")

	if _, _, err := m.Finish(id, "user2"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for foreign user, got %v", err)
	}
}

func TestManager_FinishIsOneShot(t *testing.T) {
	m := NewManager(ManagerConfig{})
	id := m.Start("user1", "zh")

	if _, _, err := m.Finish(id, "user1"); err != nil {
		t.Fatalf("finish: %v", err)
	}
	// Second finish → session already removed.
	if _, _, err := m.Finish(id, "user1"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound on second finish, got %v", err)
	}
}

func TestManager_EvictStale(t *testing.T) {
	m := NewManager(ManagerConfig{TTL: 10 * time.Millisecond})
	id := m.Start("user1", "zh")

	time.Sleep(20 * time.Millisecond)
	m.evictStale()

	if _, _, err := m.Finish(id, "user1"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after eviction, got %v", err)
	}
}

func TestManager_StartCleanupStops(t *testing.T) {
	m := NewManager(ManagerConfig{TTL: time.Hour})
	stop := m.StartCleanup()
	stop() // must not panic on double-stop race
	stop()
}
