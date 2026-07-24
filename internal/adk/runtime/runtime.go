// Package adkruntime assembles the ADK llmagent + runner used by the chat
// and agent services. It replaces the legacy hand-written Engine with
// ADK's built-in ReAct loop, session persistence, compaction, and memory.
package adkruntime

import (
	"context"
	"fmt"
	"iter"
	"strings"

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
	AuditInput(input string) error
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
}

// Runtime wraps an ADK runner bound to one agent.
type Runtime struct {
	runner  *runner.Runner
	appName string
}

// DefaultInstruction is the agent's system prompt.
const DefaultInstruction = `You are a data analysis agent. Help the user analyze data, query knowledge bases, compute statistics, and produce reports.
Use the available tools when they help answer the question. Answer in the same language the user uses.`

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
	if cfg.Instruction == "" {
		cfg.Instruction = DefaultInstruction
	}

	agentCfg := llmagent.Config{
		Name:        "data_agent",
		Description: "Enterprise data analysis agent",
		Model:       cfg.Model,
		Instruction: cfg.Instruction,
		Tools:       cfg.Tools,
	}
	if cfg.Auditor != nil {
		agentCfg.BeforeModelCallbacks = []llmagent.BeforeModelCallback{auditInputCallback(cfg.Auditor)}
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
	return &Runtime{runner: r, appName: cfg.AppName}, nil
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
	msg := genai.NewContentFromText(message, "user")
	agentCfg := agent.RunConfig{StreamingMode: agent.StreamingModeNone}
	if rc.Streaming {
		agentCfg.StreamingMode = agent.StreamingModeSSE
	}

	var opts []runner.RunOption
	if len(rc.StateDelta) > 0 {
		opts = append(opts, runner.WithStateDelta(rc.StateDelta))
	}
	return rt.runner.Run(ctx, userID, sessionID, msg, agentCfg, opts...)
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
	var finalText strings.Builder
	runErr := error(nil)
	for evt, err := range rt.Run(ctx, userID, sessionID, message, rc) {
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

// auditPart audits a single text part.
func auditPart(a Auditor, p *genai.Part) error {
	if p == nil || p.Text == "" {
		return nil
	}
	if err := a.AuditInput(p.Text); err != nil {
		return fmt.Errorf("input audit failed: %w", err)
	}
	return nil
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
