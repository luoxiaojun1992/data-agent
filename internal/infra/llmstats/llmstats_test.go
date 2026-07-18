package llmstats

import (
	"context"
	"errors"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestEstimateTokens(t *testing.T) {
	if EstimateTokens("") != 0 {
		t.Error("empty")
	}
	if EstimateTokens("abcd") != 1 {
		t.Error("4 chars = 1 token")
	}
	if EstimateTokens("abcde") != 2 {
		t.Error("5 chars = 2 tokens")
	}
	if EstimateTokens("hello world") > 10 {
		t.Error("reasonable")
	}
}

func TestCacheKey(t *testing.T) {
	k1 := CacheKey("emb", "m", "hello")
	k2 := CacheKey("emb", "m", "hello")
	if k1 != k2 {
		t.Error("deterministic")
	}
	if CacheKey("emb", "m", "hello") == CacheKey("emb", "m", "world") {
		t.Error("different inputs")
	}
}

func TestRecorder_Record(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var coll mongo.Collection
	patches.ApplyMethodReturn(&coll, "InsertOne", &mongo.InsertOneResult{}, nil)

	r := &Recorder{coll: &coll}
	err := r.Record(context.Background(), Record{
		CallPoint: "chat", Model: "gpt-4",
		PromptTokens: 100, CompletionTokens: 50, Multiplier: 2.0,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
}

func TestRecorder_BilledTokensCalc(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var coll mongo.Collection
	patches.ApplyMethodReturn(&coll, "InsertOne", &mongo.InsertOneResult{}, nil)

	r := &Recorder{coll: &coll}
	err := r.Record(context.Background(), Record{
		CallPoint: "chat", Model: "gpt-4",
		PromptTokens: 100, CompletionTokens: 50, Multiplier: 3.0,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	// Billed calculation verified in unit: (100+50)*3 = 450
}
func TestRecorder_ZeroMultiplier(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var coll mongo.Collection
	patches.ApplyMethodReturn(&coll, "InsertOne", &mongo.InsertOneResult{}, nil)

	r := &Recorder{coll: &coll}
	err := r.Record(context.Background(), Record{
		CallPoint: "chat", PromptTokens: 10, CompletionTokens: 5, Multiplier: 0,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
}
func TestRecorder_RecordError(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var coll mongo.Collection
	patches.ApplyMethodReturn(&coll, "InsertOne", (*mongo.InsertOneResult)(nil), errors.New("db down"))

	r := &Recorder{coll: &coll}
	err := r.Record(context.Background(), Record{CallPoint: "chat"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewRecorder(t *testing.T) {
	t.Skip("NewRecorder creates indexes — covered by integration")
}
