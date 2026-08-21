package modelcfg

import (
	"context"
	"fmt"
	"iter"
	"sync"

	"google.golang.org/adk/model"
)

// LazyLLM wraps a per-use-case system LLM whose backend is built lazily on
// first use instead of at startup. If the model is not yet configured (e.g. on
// the very first boot before any model is seeded), it fails silently — the
// call yields an error rather than panicking or aborting startup — and retries
// building on the next call. Once a build succeeds, the backend is cached and
// reused as a singleton.
type LazyLLM struct {
	provider *Provider
	useCase  UseCase

	mu    sync.Mutex
	built model.LLM // nil until the first successful build
}

// NewLazyLLM creates a lazily-built, singleton LLM for the given use case.
func NewLazyLLM(provider *Provider, useCase UseCase) *LazyLLM {
	return &LazyLLM{provider: provider, useCase: useCase}
}

// resolve returns the built backend, building it on first successful use.
// It never panics: when the model is unavailable it returns nil so callers
// degrade gracefully. Subsequent calls retry until a build succeeds, after
// which the backend is cached (singleton).
func (l *LazyLLM) resolve(ctx context.Context) model.LLM {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.built != nil {
		return l.built
	}
	if l.provider == nil {
		return nil
	}
	llm, err := l.provider.BuildLLM(ctx, l.useCase)
	if err != nil {
		return nil
	}
	l.built = llm
	return l.built
}

// Name returns the backend model name once built, or the use-case name while
// unbuilt so callers never observe an empty name.
func (l *LazyLLM) Name() string {
	if b := l.resolve(context.Background()); b != nil {
		return b.Name()
	}
	return string(l.useCase)
}

// GenerateContent forwards to the lazily-built backend, or yields a single
// error when the model is not yet available.
func (l *LazyLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	b := l.resolve(ctx)
	if b == nil {
		return func(yield func(*model.LLMResponse, error) bool) {
			yield(nil, fmt.Errorf("modelcfg: lazy LLM %q not available (no model configured)", l.useCase))
		}
	}
	return b.GenerateContent(ctx, req, stream)
}
