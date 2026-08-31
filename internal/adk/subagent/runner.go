// Package subagent implements the SPEC-071 sub-agent tool. The parent agent
// delegates a multi-step subtask to an independent sub-agent — same model as
// the parent session, trimmed tool set (no sub-agent tool itself, preventing
// recursive delegation) — which runs on its own parent-bound session that is
// destroyed on completion. The sub-agent's final text is returned as the tool
// result, which ADK wraps as a FunctionResponse written back to the parent.
//
// Import direction is one-way: subagent → adkruntime → (modelcfg, security).
// adkruntime registers tools via the `tool.Tool` interface and never imports a
// tool implementation package, so no Go import cycle exists.
package subagent

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"google.golang.org/adk/session"

	adkruntime "github.com/luoxiaojun1992/data-agent/internal/adk/runtime"
	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
)

// SessionService is the ADK session service plus sub-session support. The
// concrete *adksession.Service satisfies it (CreateSubSession adds the parent
// binding used for cascade deletion).
type SessionService interface {
	session.Service
	CreateSubSession(ctx context.Context, req *session.CreateRequest, parentSessionID string) (*session.CreateResponse, error)
}

// ParentResolver resolves a parent session's bound model ID (requirement ⑥:
// the sub-agent reuses the parent's model). The concrete *chat.Manager
// satisfies it via Get.
type ParentResolver interface {
	Get(id string) (*domainchat.Session, error)
}

// Runner launches independent sub-agent runs (SPEC-071).
type Runner struct {
	registry *adkruntime.Registry
	sessions SessionService
	parent   ParentResolver
	appName  string
}

// NewRunner wires the sub-agent runner. registry supplies the sub-agent
// Runtime (same model + trimmed tools); sessions creates/destroys independent
// sub sessions; parent resolves the parent session's bound model ID.
func NewRunner(registry *adkruntime.Registry, sessions SessionService, parent ParentResolver) *Runner {
	return &Runner{
		registry: registry,
		sessions: sessions,
		parent:   parent,
		appName:  registry.AppName(),
	}
}

// Run delegates task to a sub-agent bound to parentSessionID. The sub-agent:
//  1. reuses the parent session's model (resolved via the business session);
//  2. runs on an independent parent-bound session (independent history);
//  3. inherits ctx — a cancelled parent ctx cancels the sub-agent run;
//  4. destroys its session (DB + raw events) on completion or cancellation.
//
// It returns the sub-agent's final text. The sub session's tool state carries
// the parent session ID so file/artifact side effects land in the parent's
// context; only the final text is returned to the parent LLM.
func (r *Runner) Run(ctx context.Context, parentSessionID, userID, task string, state map[string]any) (string, error) {
	modelID, err := r.resolveModel(parentSessionID)
	if err != nil {
		return "", err
	}

	subID := "sess_sub_" + uuid.New().String()
	if _, err := r.sessions.CreateSubSession(ctx, &session.CreateRequest{
		AppName:   r.appName,
		UserID:    userID,
		SessionID: subID,
		State:     state,
	}, parentSessionID); err != nil {
		return "", fmt.Errorf("sub agent: create sub session: %w", err)
	}

	// Cleanup always runs on a detached context so a cancelled parent cannot
	// leave the sub session behind (SPEC-071 取消清理). It also covers the
	// normal completion path (sub session destroyed right after return).
	defer r.cleanup(subID, userID)

	rt, err := r.registry.GetOrCreateSubAgent(ctx, modelID)
	if err != nil {
		return "", fmt.Errorf("sub agent: resolve runtime: %w", err)
	}

	// Sub-agent runs with the parent's ctx (cancellation inherits). The
	// independent sub session ID scopes history; StateDelta carries identity +
	// parent session ID so tools act in the parent's context.
	text, runErr := rt.RunAndCollect(ctx, userID, subID, task, adkruntime.RunConfig{StateDelta: state})
	return text, runErr
}

// resolveModel returns the parent session's bound model ID, which the
// sub-agent reuses (requirement ⑥: sub-agent model == parent model).
func (r *Runner) resolveModel(parentSessionID string) (string, error) {
	if r.parent == nil {
		return "", fmt.Errorf("sub agent: parent resolver unavailable")
	}
	parent, err := r.parent.Get(parentSessionID)
	if err != nil || parent == nil {
		return "", fmt.Errorf("sub agent: resolve parent session %q: %w", parentSessionID, err)
	}
	if parent.ModelID == "" {
		return "", fmt.Errorf("sub agent: parent session %q has no bound model", parentSessionID)
	}
	return parent.ModelID, nil
}

// cleanup hard-deletes the sub session (DB record + raw events) on a detached
// context. Best-effort; a leftover sub session is defensive-cascade-deleted
// when the parent is removed.
func (r *Runner) cleanup(subID, userID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := r.sessions.Delete(ctx, &session.DeleteRequest{
		AppName:   r.appName,
		UserID:    userID,
		SessionID: subID,
	}); err != nil {
		log.Printf("[subagent] cleanup sub session %s: %v", subID, err)
	}
}
