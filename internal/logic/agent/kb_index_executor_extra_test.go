package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"google.golang.org/adk/model"

	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
)

// buildChunkFn's non-nil provider path: it calls Provider.BuildLLM for semantic
// chunking and falls back to paragraph splitting on any failure. The LLM is
// stubbed via gomonkey + the shared fakeLLM (no real model backend).

func TestBuildChunkFn_LLMChunks(t *testing.T) {
	e := &KBIndexExecutor{provider: &modelcfg.Provider{}}
	p := gomonkey.NewPatches()
	t.Cleanup(p.Reset)
	llm := &fakeLLM{text: "chunk one\n---CHUNK---\nchunk two"}
	p.ApplyMethodFunc(&modelcfg.Provider{}, "BuildLLM",
		func(_ context.Context, _ modelcfg.UseCase) (model.LLM, error) { return llm, nil })

	fn := e.buildChunkFn(context.Background())
	chunks, err := fn("some long text")
	if err != nil {
		t.Fatalf("buildChunkFn: %v", err)
	}
	if len(chunks) != 2 || chunks[0] != "chunk one" || chunks[1] != "chunk two" {
		t.Errorf("chunks = %v, want [chunk one chunk two]", chunks)
	}
}

func TestBuildChunkFn_LLMErrorFallsBack(t *testing.T) {
	e := &KBIndexExecutor{provider: &modelcfg.Provider{}}
	p := gomonkey.NewPatches()
	t.Cleanup(p.Reset)
	p.ApplyMethodFunc(&modelcfg.Provider{}, "BuildLLM",
		func(_ context.Context, _ modelcfg.UseCase) (model.LLM, error) { return nil, errors.New("no model") })

	fn := e.buildChunkFn(context.Background())
	chunks, err := fn("first\n\nsecond")
	if err != nil {
		t.Fatalf("buildChunkFn fallback error = %v", err)
	}
	if len(chunks) != 1 || chunks[0] != "first\n\nsecond" {
		t.Errorf("fallback chunks = %v, want merged paragraph", chunks)
	}
}

func TestBuildChunkFn_EmptyResultFallsBack(t *testing.T) {
	e := &KBIndexExecutor{provider: &modelcfg.Provider{}}
	p := gomonkey.NewPatches()
	t.Cleanup(p.Reset)
	llm := &fakeLLM{text: "   "} // whitespace-only → parseChunkResponse returns empty
	p.ApplyMethodFunc(&modelcfg.Provider{}, "BuildLLM",
		func(_ context.Context, _ modelcfg.UseCase) (model.LLM, error) { return llm, nil })

	fn := e.buildChunkFn(context.Background())
	chunks, err := fn("first\n\nsecond")
	if err != nil {
		t.Fatalf("buildChunkFn fallback error = %v", err)
	}
	if len(chunks) != 1 || chunks[0] != "first\n\nsecond" {
		t.Errorf("fallback chunks = %v, want merged paragraph", chunks)
	}
}
