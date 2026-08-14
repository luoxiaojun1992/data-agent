// Package adkruntime assembles the ADK llmagent + runner used by the chat
// and agent services. The Registry (this file) maintains a pool of Runtime
// instances keyed by model ID (session-level) and by UseCase (system-level),
// with lazy creation and fingerprint-based hot-reload (SPEC-062).
package adkruntime

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/adk/memory"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"

	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
)

// cachedRuntime holds a Runtime instance alongside the config fingerprint it
// was built from. When the fingerprint changes (admin edited the model
// config), the next GetOrCreate call rebuilds the Runtime.
type cachedRuntime struct {
	rt         *Runtime
	hash       string       // sha256 of the model config at build time
	lastAccess atomic.Int64 // unix nano, updated on every GetOrCreate hit
}

const runtimeTTL = 30 * time.Minute

func (cr *cachedRuntime) touch()           { cr.lastAccess.Store(time.Now().UnixNano()) }
func (cr *cachedRuntime) stale() bool       { return time.Since(time.Unix(0, cr.lastAccess.Load())) > runtimeTTL }

// ModelProvider abstracts the model-config reads the Registry needs. The
// concrete *modelcfg.Provider satisfies it; tests inject a mock to drive
// cache-hit / lazy-create / fingerprint-rebuild scenarios without a database.
type ModelProvider interface {
	GetModelByID(ctx context.Context, modelID string) (*modelcfg.ModelEntry, error)
	GetModelByUseCase(ctx context.Context, useCase modelcfg.UseCase) (*modelcfg.ModelEntry, error)
	BuildLLMByID(ctx context.Context, modelID string) (model.LLM, error)
	DefaultInstruction(ctx context.Context) string
}

// RegistryConfig carries the shared dependencies used to build every Runtime.
// Tools, Auditor, SessionService, and MemoryService are shared across all
// Runtime instances; only Model and Instruction differ per entry.
type RegistryConfig struct {
	Provider       ModelProvider
	SessionService session.Service
	MemoryService  memory.Service
	Tools          []tool.Tool
	Auditor        Auditor
	AppName        string
}

// Registry maintains three caches of Runtime instances:
//   - sessions: keyed by model ID (ModelEntry.ID) — per-model Runtime shared
//     by all sessions bound to that model (lazy create + fingerprint hot-reload).
//   - sys: keyed by UseCase — system-level Runtime for enhance/compaction/
//     memoryx scenarios that don't follow the user's session-bound model.
//   - taskInst: keyed by (modelID + instructionSuffix) — task-mode runtimes
//     that carry a caller-supplied instruction override (e.g. the
//     save_task_result reminder). Kept separate so task instructions don't
//     pollute the default chat Runtime pool.
//
// All paths lazily create Runtime instances on first use and rebuild them
// when the underlying model config fingerprint changes (no Pub/Sub needed).
type Registry struct {
	cfg      RegistryConfig
	mu       sync.RWMutex
	sessions map[string]*cachedRuntime       // key = ModelEntry.ID
	sys      map[modelcfg.UseCase]*cachedRuntime
	taskInst map[string]*cachedRuntime       // key = entry.ID + "|" + instructionSuffix
}

// NewRegistry creates an empty Runtime registry. Runtimes are created lazily
// on the first GetOrCreate / GetOrCreateByUseCase call.
func NewRegistry(cfg RegistryConfig) *Registry {
	return &Registry{
		cfg:      cfg,
		sessions: make(map[string]*cachedRuntime),
		sys:      make(map[modelcfg.UseCase]*cachedRuntime),
		taskInst: make(map[string]*cachedRuntime),
	}
}

// GetOrCreate returns the session-level Runtime for the given model ID. When
// modelID is empty, the default LLM model is used (Provider.DefaultModel).
// The Runtime is lazily created on first use and rebuilt when the model
// config changes (fingerprint mismatch), giving hot-reload without Pub/Sub.
// Concurrent calls for the same modelID create the Runtime exactly once.
//
// Stored in the sessions cache. Use GetOrCreateWithInstruction to inject a
// per-call instruction override (task-mode runs) — that path stores in the
// separate taskInst cache so the default chat pool isn't polluted.
func (r *Registry) GetOrCreate(ctx context.Context, modelID string) (*Runtime, error) {
	entry, err := r.cfg.Provider.GetModelByID(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("resolve model %q: %w", modelID, err)
	}
	hash := modelcfg.ConfigHash(*entry)

	// Fast path: read lock, reuse if fingerprint matches.
	r.mu.RLock()
	if cached, ok := r.sessions[entry.ID]; ok && cached.hash == hash {
		cached.touch()
		rt := cached.rt
		r.mu.RUnlock()
		return rt, nil
	}
	r.mu.RUnlock()

	// Slow path: create/rebuild under write lock with double-check.
	r.mu.Lock()
	defer r.mu.Unlock()
	if cached, ok := r.sessions[entry.ID]; ok && cached.hash == hash {
		cached.touch()
		return cached.rt, nil
	}
	rt, err := r.buildRuntime(ctx, entry.ID, entry.Instruction)
	if err != nil {
		return nil, err
	}
	cr := &cachedRuntime{rt: rt, hash: hash}
	cr.touch()
	r.sessions[entry.ID] = cr
	return rt, nil
}

// GetOrCreateWithInstruction returns the session-level Runtime for the given
// model ID with a caller-supplied instruction override. The override is
// appended to the model's configured instruction (or used as-is when the
// model has none). The (modelID, instructionSuffix) pair is the cache key, so
// different task prompts get distinct Runtime instances and don't pollute
// the default pool.
//
// Used by AgentExecutor to inject task-specific instructions (e.g. the
// save_task_result reminder for async/scheduled task runs) without forcing
// every chat session to inherit that suffix.
//
// Empty instructionSuffix behaves identically to GetOrCreate.
func (r *Registry) GetOrCreateWithInstruction(ctx context.Context, modelID, instructionSuffix string) (*Runtime, error) {
	entry, err := r.cfg.Provider.GetModelByID(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("resolve model %q: %w", modelID, err)
	}
	hash := modelcfg.ConfigHash(*entry)

	var instruction string
	if instructionSuffix != "" {
		// Task mode: base = model instruction (fallback to the default
		// instruction when empty), then append the suffix. This keeps the
		// data-analysis workflow prompt even for models without a custom
		// instruction, and guarantees save_task_result guidance on top.
		base := entry.Instruction
		if base == "" {
			base = modelcfg.DefaultInstruction
		}
		instruction = base + "\n\n" + instructionSuffix
	} else {
		instruction = entry.Instruction
	}
	instKey := entry.ID + "|" + instructionSuffix

	// Fast path: read lock, reuse if fingerprint matches.
	r.mu.RLock()
	if cached, ok := r.taskInst[instKey]; ok && cached.hash == hash {
		cached.touch()
		rt := cached.rt
		r.mu.RUnlock()
		return rt, nil
	}
	r.mu.RUnlock()

	// Slow path: create/rebuild under write lock with double-check.
	r.mu.Lock()
	defer r.mu.Unlock()
	if cached, ok := r.taskInst[instKey]; ok && cached.hash == hash {
		cached.touch()
		return cached.rt, nil
	}
	rt, err := r.buildRuntime(ctx, entry.ID, instruction)
	if err != nil {
		return nil, err
	}
	cr := &cachedRuntime{rt: rt, hash: hash}
	cr.touch()
	r.taskInst[instKey] = cr
	return rt, nil
}

// GetOrCreateByUseCase returns the system-level Runtime for the given use
// case (e.g. enhance, compaction). Each use case gets its own Runtime
// instance so different system scenarios can use different (e.g. cheaper)
// models. Like the session path, instances are lazily created and rebuilt
// on config change.
func (r *Registry) GetOrCreateByUseCase(ctx context.Context, useCase modelcfg.UseCase) (*Runtime, error) {
	entry, err := r.cfg.Provider.GetModelByUseCase(ctx, useCase)
	if err != nil {
		return nil, fmt.Errorf("resolve model for use case %q: %w", useCase, err)
	}
	hash := modelcfg.ConfigHash(*entry)

	r.mu.RLock()
	if cached, ok := r.sys[useCase]; ok && cached.hash == hash {
		cached.touch()
		rt := cached.rt
		r.mu.RUnlock()
		return rt, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if cached, ok := r.sys[useCase]; ok && cached.hash == hash {
		cached.touch()
		return cached.rt, nil
	}
	rt, err := r.buildRuntime(ctx, entry.ID, entry.Instruction)
	if err != nil {
		return nil, err
	}
	cr := &cachedRuntime{rt: rt, hash: hash}
	cr.touch()
	r.sys[useCase] = cr
	return rt, nil
}

// buildRuntime constructs a new Runtime for the given model ID. Shared
// dependencies (tools, auditor, session/memory services) come from the
// registry config; only Model and Instruction are model-specific.
func (r *Registry) buildRuntime(ctx context.Context, modelID, instruction string) (*Runtime, error) {
	llm, err := r.cfg.Provider.BuildLLMByID(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("build LLM for %q: %w", modelID, err)
	}
	if instruction == "" {
		instruction = r.cfg.Provider.DefaultInstruction(ctx)
	}
	return New(Config{
		AppName:        r.cfg.AppName,
		Model:          llm,
		SessionService: r.cfg.SessionService,
		MemoryService:  r.cfg.MemoryService,
		Tools:          r.cfg.Tools,
		Auditor:        r.cfg.Auditor,
		Instruction:    instruction,
	})
}

// AppName returns the shared app name used by every Runtime in this registry.
func (r *Registry) AppName() string {
	return r.cfg.AppName
}

// StartCleanup runs a background goroutine that periodically evicts stale
// Runtime entries (lastAccess > runtimeTTL). Returns a stop function.
func (r *Registry) StartCleanup() func() {
	ticker := time.NewTicker(5 * time.Minute)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				r.evictStale()
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()
	return func() { close(done) }
}

func (r *Registry) evictStale() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, cr := range r.sessions {
		if cr.stale() {
			delete(r.sessions, id)
		}
	}
	for uc, cr := range r.sys {
		if cr.stale() {
			delete(r.sys, uc)
		}
	}
	for key, cr := range r.taskInst {
		if cr.stale() {
			delete(r.taskInst, key)
		}
	}
}
