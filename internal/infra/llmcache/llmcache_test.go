package llmcache

import (
	"encoding/json"
	"testing"
)

func TestCacheKey(t *testing.T) {
	k1 := embedKey("m", "hello")
	k2 := embedKey("m", "hello")
	if k1 != k2 {
		t.Error("deterministic embedKey")
	}
	if embedKey("m1", "hello") == embedKey("m2", "hello") {
		t.Error("different models produce different keys")
	}
	if enhanceKey("m", "hi") == enhanceKey("m", "hello") {
		t.Error("different inputs produce different keys")
	}
}

func TestMarshalEntry(t *testing.T) {
	entry := marshalEntry("test result")
	var ce CacheEntry
	if err := json.Unmarshal([]byte(entry), &ce); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ce.Result != "test result" {
		t.Errorf("Result = %q", ce.Result)
	}
}

func TestNew(t *testing.T) {
	c := New(nil)
	if c == nil {
		t.Error("nil client should still return Cache")
	}
}
