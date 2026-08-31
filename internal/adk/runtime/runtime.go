// Package adkruntime assembles the ADK llmagent + runner used by the chat
// and agent services. It replaces the legacy hand-written Engine with
// ADK's built-in ReAct loop, session persistence, compaction, and memory.
package adkruntime

import (
	"context"
	"fmt"
	"iter"
	"log"
	"strings"
	"time"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/model"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"

	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
)

// Auditor abstracts the security auditor used by the runtime callbacks.
type Auditor interface {
	AuditInput(input string) (string, error)
	AuditOutput(output string) (string, error)
	AuditToolCall(toolName string, params map[string]any) error
}

// Config configures a Runtime.
type Config struct {
	// AppName namespaces sessions and memory entries.
	AppName string
	// Model is the LLM (or fallback chain) used by the agent. Required.
	Model model.LLM
	// SessionService persists sessions. Required.
	SessionService session.Service
	// MemoryService provides long-term memory. Optional.
	MemoryService memory.Service
	// Tools are exposed to the agent.
	Tools []tool.Tool
	// Auditor runs security checks on input/output/tool calls. Optional.
	Auditor Auditor
	// Instruction is the system prompt for the agent.
	Instruction string
	// MaxInputTokens is the model's max input token limit (context_len). When
	// > 0, input exceeding it is rejected before the model call (SPEC-068).
	MaxInputTokens int
}

// Runtime wraps an ADK runner bound to one agent.
type Runtime struct {
	runner  *runner.Runner
	appName string
	// sessSvc keeps the concrete session service for optional capabilities
	// (e.g. FlushStreamBuffer at turn end). Same instance as runner's.
	sessSvc session.Service
}

// TaskInstructionSuffix is appended to the agent's system prompt when an
// async/scheduled task invokes the LLM. The suffix forces the model to
// persist its work via the save_task_result tool — without that call the
// task has no result, and the executor retries the same session once.
const TaskInstructionSuffix = `## Task Mode — Mandatory Result Persistence

You are running inside an automated task (async or scheduled). The task
orchestrator expects you to finish by calling the **save_task_result**
function tool with a non-empty ` + "`content`" + ` argument.

**You must INVOKE the save_task_result tool, not just write a text response.**
A text response is NOT saved. The save_task_result call is what persists
the final answer and marks the task as completed.

Without the save_task_result call:
- The task has no result and the system retries you once with the same
  conversation history in this session.
- A second failure marks the task as failed in the dashboard.

At the end of your analysis, call save_task_result with the final answer
(or a summary) as ` + "`content`" + `. The content is what the user will see,
so make it complete and self-contained.

If something goes wrong mid-task (DB error, model limit, missing data,
etc.) call save_task_result with ` + "`status='failed'`" + ` and content
describing the failure — this still surfaces the issue to the user.`

// New builds the ADK agent and runner.
func New(cfg Config) (*Runtime, error) {
	if cfg.Model == nil {
		return nil, fmt.Errorf("model is required")
	}
	if cfg.SessionService == nil {
		return nil, fmt.Errorf("session service is required")
	}
	if cfg.AppName == "" {
		cfg.AppName = "data-agent"
	}

	agentCfg := llmagent.Config{
		Name:        "data_agent",
		Description: "Enterprise data analysis agent",
		Model:       cfg.Model,
		Instruction: cfg.Instruction,
		Tools:       cfg.Tools,
	}
	if cfg.MaxInputTokens > 0 {
		agentCfg.BeforeModelCallbacks = []llmagent.BeforeModelCallback{maxInputTokensCallback(cfg.MaxInputTokens)}
	}
	if cfg.Auditor != nil {
		agentCfg.BeforeModelCallbacks = append(agentCfg.BeforeModelCallbacks, auditInputCallback(cfg.Auditor))
		agentCfg.AfterModelCallbacks = []llmagent.AfterModelCallback{auditOutputCallback(cfg.Auditor)}
		agentCfg.BeforeToolCallbacks = []llmagent.BeforeToolCallback{auditToolCallCallback(cfg.Auditor)}
	}

	a, err := llmagent.New(agentCfg)
	if err != nil {
		return nil, fmt.Errorf("create llm agent: %w", err)
	}

	r, err := runner.New(runner.Config{
		AppName:        cfg.AppName,
		Agent:          a,
		SessionService: cfg.SessionService,
		MemoryService:  cfg.MemoryService,
	})
	if err != nil {
		return nil, fmt.Errorf("create runner: %w", err)
	}
	return &Runtime{runner: r, appName: cfg.AppName, sessSvc: cfg.SessionService}, nil
}

// RunConfig controls a single run.
type RunConfig struct {
	// Streaming requests SSE streaming from the model backend.
	Streaming bool
	// StateDelta carries session state values (user_id, role, kb_id) injected
	// before the run so tools can read them via tool.Context.State().
	StateDelta map[string]any
}

// Run executes one conversation turn and returns the event stream.
func (rt *Runtime) Run(ctx context.Context, userID, sessionID, message string, rc RunConfig) iter.Seq2[*session.Event, error] {
	return rt.RunContent(ctx, userID, sessionID, genai.NewContentFromText(message, "user"), rc)
}

// RunContent executes one conversation turn with a caller-built user content
// (text plus optional image parts) and returns the event stream. Chat image
// attachments flow through this path; the string-only Run above delegates here.
func (rt *Runtime) RunContent(ctx context.Context, userID, sessionID string, content *genai.Content, rc RunConfig) iter.Seq2[*session.Event, error] {
	agentCfg := agent.RunConfig{StreamingMode: agent.StreamingModeNone}
	if rc.Streaming {
		agentCfg.StreamingMode = agent.StreamingModeSSE
	}

	var opts []runner.RunOption
	if len(rc.StateDelta) > 0 {
		opts = append(opts, runner.WithStateDelta(rc.StateDelta))
	}
	inner := rt.runner.Run(ctx, userID, sessionID, content, agentCfg, opts...)
	return func(yield func(*session.Event, error) bool) {
		for evt, err := range inner {
			if !yield(evt, err) {
				rt.flushStreamBuffer(userID, sessionID)
				return
			}
		}
		// Turn end: force-flush buffered streaming chunks. The ieshan openai
		// backend's final response still carries Partial=true, so AppendEvent
		// never flushes the last assistant message on its own (SPEC-069 问题 4
		// follow-up — the transcript would otherwise miss the LLM reply).
		rt.flushStreamBuffer(userID, sessionID)
	}
}

// flushStreamBuffer asks the session service (when it supports the optional
// interface) to flush buffered streaming text into session_events. Failures
// are logged only — the turn itself already completed.
func (rt *Runtime) flushStreamBuffer(userID, sessionID string) {
	flusher, ok := rt.sessSvc.(interface {
		FlushStreamBuffer(ctx context.Context, appName, userID, sessionID string) error
	})
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := flusher.FlushStreamBuffer(ctx, rt.appName, userID, sessionID); err != nil {
		log.Printf("[runtime] flush stream buffer (session=%s): %v", sessionID, err)
	}
}

// RunAndCollect executes one ADK turn and returns the final assistant text.
// It iterates the Run event stream, collecting text from final-response events
// and surfacing the first error (breaking on it). Intermediate tool call /
// response events are consumed but not surfaced.
//
// Shared by chat.Service (real-time path) and the async AgentExecutor
// (SPEC-063) so both paths use identical collection logic — async tasks execute
// with the same semantics as a real-time chat turn.
func (rt *Runtime) RunAndCollect(ctx context.Context, userID, sessionID, message string, rc RunConfig) (string, error) {
	return rt.RunAndCollectContent(ctx, userID, sessionID, genai.NewContentFromText(message, "user"), rc)
}

// RunAndCollectContent is the content-based counterpart of RunAndCollect for
// user messages carrying image parts.
func (rt *Runtime) RunAndCollectContent(ctx context.Context, userID, sessionID string, content *genai.Content, rc RunConfig) (string, error) {
	var finalText strings.Builder
	runErr := error(nil)
	for evt, err := range rt.RunContent(ctx, userID, sessionID, content, rc) {
		if err != nil {
			runErr = err
			break
		}
		if evt == nil || evt.Content == nil || !evt.IsFinalResponse() {
			continue
		}
		for _, p := range evt.Content.Parts {
			if p != nil && p.Text != "" {
				finalText.WriteString(p.Text)
			}
		}
	}
	if runErr != nil {
		return "", runErr
	}
	return finalText.String(), nil
}

// AppName returns the configured app name.
func (rt *Runtime) AppName() string { return rt.appName }

// ---- auditor callbacks ----

// auditInputCallback audits the last user message before each model call.
func auditInputCallback(a Auditor) llmagent.BeforeModelCallback {
	return func(ctx agent.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
		if req == nil {
			return nil, nil
		}
		for _, c := range req.Contents {
			if err := auditContent(a, c); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
}

// auditContent runs input audit on user-role content parts.
func auditContent(a Auditor, c *genai.Content) error {
	if c == nil || c.Role != "user" {
		return nil
	}
	for _, p := range c.Parts {
		if err := auditPart(a, p); err != nil {
			return err
		}
	}
	return nil
}

// auditPart audits a single text part, writing the redacted text back.
func auditPart(a Auditor, p *genai.Part) error {
	if p == nil || p.Text == "" {
		return nil
	}
	sanitized, err := a.AuditInput(p.Text)
	if err != nil {
		return fmt.Errorf("input audit failed: %w", err)
	}
	p.Text = sanitized
	return nil
}

// maxInputTokensCallback rejects inputs whose estimated token count exceeds
// the model's context_len (SPEC-068), avoiding a wasted model call. It runs
// before auditInputCallback so over-long input is rejected without redaction.
func maxInputTokensCallback(limit int) llmagent.BeforeModelCallback {
	return func(ctx agent.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
		if req == nil || limit <= 0 {
			return nil, nil
		}
		for _, c := range req.Contents {
			if c == nil {
				continue
			}
			for _, p := range c.Parts {
				if p == nil {
					continue
				}
				if est := estimateTokens(p.Text); est > limit {
					return nil, fmt.Errorf("input exceeds model max input tokens (%d > %d)", est, limit)
				}
			}
		}
		return nil, nil
	}
}

// estimateTokens approximates the token count of text (SPEC-068). It uses a
// lightweight rune-based heuristic (≈4 chars/token) to avoid a heavy tokenizer;
// the estimate is allowed to be slightly conservative (reject early).
func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	n := len([]rune(text))
	if n < 4 {
		return 1
	}
	return (n + 3) / 4
}

// auditOutputCallback sanitizes model output text in place.
func auditOutputCallback(a Auditor) llmagent.AfterModelCallback {
	return func(ctx agent.CallbackContext, resp *model.LLMResponse, respErr error) (*model.LLMResponse, error) {
		if respErr != nil || resp == nil || resp.Content == nil {
			return resp, respErr
		}
		for _, p := range resp.Content.Parts {
			if p == nil || p.Text == "" {
				continue
			}
			sanitized, err := a.AuditOutput(p.Text)
			if err != nil {
				return nil, fmt.Errorf("output audit failed: %w", err)
			}
			p.Text = sanitized
		}
		return resp, nil
	}
}

// auditToolCallCallback audits tool calls before execution.
func auditToolCallCallback(a Auditor) llmagent.BeforeToolCallback {
	return func(ctx agent.ToolContext, t tool.Tool, args map[string]any) (map[string]any, error) {
		if err := a.AuditToolCall(t.Name(), args); err != nil {
			return nil, fmt.Errorf("tool call audit failed for %q: %w", t.Name(), err)
		}
		return nil, nil
	}
}

// ensure security.Auditor satisfies the Auditor interface at compile time.
var _ Auditor = (*security.Auditor)(nil)
