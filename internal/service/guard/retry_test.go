package guard

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"

	"github.com/luoxiaojun1992/data-agent/internal/infra/redis"
)

// RecordAndShouldRetry owns the bounded retry counter: first failure sets a TTL,
// reaching maxRetries resets the counter and stops retrying. These branches are
// exercised by patching the concrete *redis.Client methods.

func TestRecordAndShouldRetry_RetryAllowed(t *testing.T) {
	rc := &redis.Client{}
	var expireKey string
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(rc, "Incr",
		func(_ context.Context, key string) (int64, error) { return 1, nil })
	patches.ApplyMethodFunc(rc, "Expire",
		func(_ context.Context, key string, ttl time.Duration) error { expireKey = key; return nil })

	s := NewService(nil, nil, 2)
	s.SetRedis(rc)
	ok, err := s.RecordAndShouldRetry(context.Background(), "sess_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("first failure should allow a retry")
	}
	if expireKey != "guard:relevance:sess_1" {
		t.Errorf("Expire key = %q, want guard:relevance:sess_1 (first increment must set TTL)", expireKey)
	}
}

func TestRecordAndShouldRetry_StopAtMax(t *testing.T) {
	rc := &redis.Client{}
	var delKey string
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(rc, "Incr",
		func(_ context.Context, key string) (int64, error) { return 2, nil })
	patches.ApplyMethodFunc(rc, "Del",
		func(_ context.Context, keys ...string) error {
			if len(keys) > 0 {
				delKey = keys[0]
			}
			return nil
		})

	s := NewService(nil, nil, 2)
	s.SetRedis(rc)
	ok, err := s.RecordAndShouldRetry(context.Background(), "sess_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("reaching maxRetries should stop retrying")
	}
	if delKey != "guard:relevance:sess_1" {
		t.Errorf("Del key = %q, want counter reset on reaching max", delKey)
	}
}

func TestRecordAndShouldRetry_IncrError(t *testing.T) {
	rc := &redis.Client{}
	patches := gomonkey.ApplyMethodFunc(rc, "Incr",
		func(_ context.Context, key string) (int64, error) { return 0, errors.New("redis down") })
	t.Cleanup(patches.Reset)

	s := NewService(nil, nil, 2)
	s.SetRedis(rc)
	if _, err := s.RecordAndShouldRetry(context.Background(), "sess_1"); err == nil {
		t.Fatal("expected error when Incr fails")
	}
}
