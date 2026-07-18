package adkmodel

import (
	"context"
	"fmt"
	"iter"
	"strings"

	"google.golang.org/adk/model"
)

// FallbackLLM implements model.LLM as an ordered chain of backends.
// On backend failure it transparently retries the next one, providing
// the multi-provider fallback routing that replaces the legacy Router.
type FallbackLLM struct {
	models []model.LLM
	name   string
}

// NewFallbackLLM creates a fallback chain. The first model is primary.
// At least one model is required.
func NewFallbackLLM(models ...model.LLM) (*FallbackLLM, error) {
	if len(models) == 0 {
		return nil, fmt.Errorf("fallback chain requires at least one model")
	}
	names := make([]string, 0, len(models))
	for _, m := range models {
		if m == nil {
			return nil, fmt.Errorf("fallback chain contains nil model")
		}
		names = append(names, m.Name())
	}
	return &FallbackLLM{models: models, name: strings.Join(names, ",")}, nil
}

// Name returns the comma-joined names of the chain, primary first.
func (f *FallbackLLM) Name() string { return f.name }

// GenerateContent tries each backend in order until one succeeds.
// A backend is only abandoned when it fails before yielding any response;
// a partial stream failure is propagated to the caller.
func (f *FallbackLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		var errs []string
		for _, m := range f.models {
			done, tryNext := f.runBackend(ctx, m, req, stream, yield, &errs)
			if done {
				return
			}
			if !tryNext {
				return
			}
		}
		yield(nil, fmt.Errorf("all %d model backends failed: %s", len(f.models), strings.Join(errs, "; ")))
	}
}

// runBackend executes one backend. It returns (done=true) when the caller
// should stop iterating (success or consumer break or mid-stream failure),
// and (tryNext=true) when the backend failed before producing any response.
func (f *FallbackLLM) runBackend(
	ctx context.Context,
	m model.LLM,
	req *model.LLMRequest,
	stream bool,
	yield func(*model.LLMResponse, error) bool,
	errs *[]string,
) (done, tryNext bool) {
	yielded := false
	for resp, err := range m.GenerateContent(ctx, req, stream) {
		if err != nil {
			if yielded {
				// Mid-stream failure — propagate, never fall back.
				yield(nil, err)
				return true, false
			}
			*errs = append(*errs, fmt.Sprintf("%s: %v", m.Name(), err))
			return false, true
		}
		yielded = true
		if !yield(resp, nil) {
			return true, false // consumer stopped early
		}
	}
	return true, false // backend completed successfully
}
