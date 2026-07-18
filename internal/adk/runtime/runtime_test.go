package adkruntime

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"testing"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/genai"
)

// scriptedLLM plays back a queue of responses (text or function call).
type scriptedLLM struct {
	name  string
	queue []*model.LLMResponse
	err   error
	calls []*model.LLMRequest
}

func (s *scriptedLLM) Name() string { return s.name }

func (s *scriptedLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		s.calls = append(s.calls, req)
		if s.err != nil {
			yield(nil, s.err)
			return
		}
		if len(s.queue) == 0 {
			yield(nil, fmt.Errorf("no scripted response"))
			return
		}
		resp := s.queue[0]
		s.queue = s.queue[1:]
		yield(resp, nil)
	}
}

func textResp(text string) *model.LLMResponse {
	return &model.LLMResponse{Content: genai.NewContentFromText(text, "model")}
}

func fcResp(name string, args map[string]any) *model.LLMResponse {
	return &model.LLMResponse{Content: &genai.Content{
		Role:  "model",
		Parts: []*genai.Part{{FunctionCall: &genai.FunctionCall{Name: name, Args: args}}},
	}}
}

// drainEvents consumes an event stream, returning final texts and any error.
func drainEvents(seq iter.Seq2[*session.Event, error]) ([]string, error) {
	var texts []string
	var runErr error
	for evt, err := range seq {
		if err != nil {
			runErr = err
			break
		}
		if evt == nil || evt.Content == nil || !evt.IsFinalResponse() {
			continue
		}
		for _, p := range evt.Content.Parts {
			if p != nil && p.Text != "" {
				texts = append(texts, p.Text)
			}
		}
	}
	return texts, runErr
}

func newRuntime(t *testing.T, llm model.LLM, auditor Auditor, tools []tool.Tool) *Runtime {
	t.Helper()
	sessions := session.InMemoryService()
	rt, err := New(Config{
		AppName:        "test-app",
		Model:          llm,
		SessionService: sessions,
		Tools:          tools,
		Auditor:        auditor,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	// Pre-create the session (mirrors the chat service flow).
	if _, err := sessions.Create(context.Background(), &session.CreateRequest{
		AppName:   "test-app",
		UserID:    "u1",
		SessionID: "s1",
		State:     map[string]any{"user_id": "u1", "role": "admin"},
	}); err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	return rt
}

func TestNew_Validation(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Error("missing model should fail")
	}
	if _, err := New(Config{Model: &scriptedLLM{}}); err == nil {
		t.Error("missing session service should fail")
	}
	rt, err := New(Config{
		Model:          &scriptedLLM{name: "m"},
		SessionService: session.InMemoryService(),
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if rt.AppName() != "data-agent" {
		t.Errorf("default app name = %q", rt.AppName())
	}
}

func TestRun_SimpleText(t *testing.T) {
	llm := &scriptedLLM{name: "m", queue: []*model.LLMResponse{textResp("你好，我是数据分析助手")}}
	rt := newRuntime(t, llm, nil, nil)

	texts, err := drainEvents(rt.Run(context.Background(), "u1", "s1", "你好", RunConfig{}))
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(texts) != 1 || texts[0] != "你好，我是数据分析助手" {
		t.Errorf("texts = %v", texts)
	}
}

func TestRun_ReActLoop_ExecutesTool(t *testing.T) {
	type echoArgs struct {
		Text string `json:"text"`
	}
	var gotState string
	echoTool, err := functiontool.New(
		functiontool.Config{Name: "echo", Description: "echoes input"},
		func(tc agent.ToolContext, args echoArgs) (map[string]any, error) {
			v, _ := tc.State().Get("user_id")
			gotState, _ = v.(string)
			return map[string]any{"echo": args.Text}, nil
		},
	)
	if err != nil {
		t.Fatalf("build tool: %v", err)
	}

	llm := &scriptedLLM{name: "m", queue: []*model.LLMResponse{
		fcResp("echo", map[string]any{"text": "ping"}),
		textResp("工具结果是 pong"),
	}}
	rt := newRuntime(t, llm, nil, []tool.Tool{echoTool})

	texts, err := drainEvents(rt.Run(context.Background(), "u1", "s1", "调用 echo", RunConfig{
		StateDelta: map[string]any{"user_id": "u1"},
	}))
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// ReAct loop: LLM was called twice (function call → tool result → final answer).
	if len(llm.calls) != 2 {
		t.Fatalf("expected 2 LLM calls in ReAct loop, got %d", len(llm.calls))
	}
	// Second call must include the tool response message.
	found := false
	for _, c := range llm.calls[1].Contents {
		for _, p := range c.Parts {
			if p != nil && p.FunctionResponse != nil && p.FunctionResponse.Name == "echo" {
				found = true
			}
		}
	}
	if !found {
		t.Error("second LLM call should include the tool response")
	}
	if gotState != "u1" {
		t.Errorf("tool should read user_id from session state, got %q", gotState)
	}
	if len(texts) == 0 || texts[len(texts)-1] != "工具结果是 pong" {
		t.Errorf("final text = %v", texts)
	}
}

func TestRun_ModelError(t *testing.T) {
	llm := &scriptedLLM{name: "m", err: fmt.Errorf("model down")}
	rt := newRuntime(t, llm, nil, nil)

	_, err := drainEvents(rt.Run(context.Background(), "u1", "s1", "hi", RunConfig{}))
	if err == nil {
		t.Error("model error should propagate")
	}
}

// ---- auditor callbacks ----

type strictAuditor struct {
	inputErr   error
	outputErr  error
	toolErr    error
	sawInput   []string
	sawTool    []string
	transformO func(string) string
}

func (a *strictAuditor) AuditInput(input string) error {
	a.sawInput = append(a.sawInput, input)
	return a.inputErr
}

func (a *strictAuditor) AuditOutput(output string) (string, error) {
	if a.outputErr != nil {
		return "", a.outputErr
	}
	if a.transformO != nil {
		return a.transformO(output), nil
	}
	return output, nil
}

func (a *strictAuditor) AuditToolCall(toolName string, params map[string]any) error {
	a.sawTool = append(a.sawTool, toolName)
	return a.toolErr
}

func TestAuditor_InputBlocked(t *testing.T) {
	llm := &scriptedLLM{name: "m", queue: []*model.LLMResponse{textResp("answer")}}
	aud := &strictAuditor{inputErr: fmt.Errorf("sensitive input")}
	rt := newRuntime(t, llm, aud, nil)

	_, err := drainEvents(rt.Run(context.Background(), "u1", "s1", "my password is 123", RunConfig{}))
	if err == nil {
		t.Error("input audit failure should abort the run")
	}
}

func TestAuditor_OutputSanitized(t *testing.T) {
	llm := &scriptedLLM{name: "m", queue: []*model.LLMResponse{textResp("call 13800138000")}}
	aud := &strictAuditor{transformO: func(s string) string {
		return strings.ReplaceAll(s, "13800138000", "1**********")
	}}
	rt := newRuntime(t, llm, aud, nil)

	texts, err := drainEvents(rt.Run(context.Background(), "u1", "s1", "phone?", RunConfig{}))
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(texts) == 0 || !strings.Contains(texts[len(texts)-1], "1**********") {
		t.Errorf("output should be sanitized: %v", texts)
	}
	if len(aud.sawInput) == 0 {
		t.Error("auditor should have seen the input")
	}
}

func TestAuditor_ToolCallAudited(t *testing.T) {
	noop, err := functiontool.New(
		functiontool.Config{Name: "noop", Description: "does nothing"},
		func(tc agent.ToolContext, args map[string]any) (map[string]any, error) {
			return map[string]any{"ok": true}, nil
		},
	)
	if err != nil {
		t.Fatalf("build tool: %v", err)
	}

	llm := &scriptedLLM{name: "m", queue: []*model.LLMResponse{
		fcResp("noop", map[string]any{}),
		textResp("done"),
	}}
	aud := &strictAuditor{}
	rt := newRuntime(t, llm, aud, []tool.Tool{noop})

	if _, err := drainEvents(rt.Run(context.Background(), "u1", "s1", "go", RunConfig{})); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(aud.sawTool) != 1 || aud.sawTool[0] != "noop" {
		t.Errorf("tool call should be audited: %v", aud.sawTool)
	}
}

func TestAuditor_ToolCallBlocked(t *testing.T) {
	noop, err := functiontool.New(
		functiontool.Config{Name: "danger", Description: "dangerous"},
		func(tc agent.ToolContext, args map[string]any) (map[string]any, error) {
			return map[string]any{"ok": true}, nil
		},
	)
	if err != nil {
		t.Fatalf("build tool: %v", err)
	}

	llm := &scriptedLLM{name: "m", queue: []*model.LLMResponse{
		fcResp("danger", map[string]any{}),
		textResp("fallback answer"),
	}}
	aud := &strictAuditor{toolErr: fmt.Errorf("forbidden tool")}
	rt := newRuntime(t, llm, aud, []tool.Tool{noop})

	// Blocked tool → the run either errors or the LLM gets an error response and continues.
	_, runErr := drainEvents(rt.Run(context.Background(), "u1", "s1", "go", RunConfig{}))
	// Either outcome is acceptable; what matters is the audit callback fired.
	if len(aud.sawTool) != 1 {
		t.Errorf("audit should have fired, runErr=%v", runErr)
	}
}

func TestAuditCallbacks_NilSafety(t *testing.T) {
	aud := &strictAuditor{}

	// BeforeModelCallback with nil request.
	cb := auditInputCallback(aud)
	if _, err := cb(nil, nil); err != nil {
		t.Errorf("nil request should be safe: %v", err)
	}

	// AfterModelCallback with error passthrough.
	acb := auditOutputCallback(aud)
	respErr := fmt.Errorf("upstream")
	if _, err := acb(nil, nil, respErr); err != respErr {
		t.Errorf("error passthrough broken: %v", err)
	}

	// AfterModelCallback with nil content.
	if _, err := acb(nil, &model.LLMResponse{}, nil); err != nil {
		t.Errorf("nil content should be safe: %v", err)
	}

	// Output audit failure.
	aud.outputErr = fmt.Errorf("bad output")
	badResp := &model.LLMResponse{Content: genai.NewContentFromText("x", "model")}
	if _, err := acb(nil, badResp, nil); err == nil {
		t.Error("output audit failure should propagate")
	}
}

func TestRun_Streaming(t *testing.T) {
	llm := &scriptedLLM{name: "m", queue: []*model.LLMResponse{textResp("streamed answer")}}
	rt := newRuntime(t, llm, nil, nil)

	texts, err := drainEvents(rt.Run(context.Background(), "u1", "s1", "hi", RunConfig{Streaming: true}))
	if err != nil {
		t.Fatalf("streaming run failed: %v", err)
	}
	if len(texts) != 1 {
		t.Errorf("texts = %v", texts)
	}
}

// ensure memory.Service can be nil and set — compile-time wiring check.
func TestNew_WithMemoryService(t *testing.T) {
	rt, err := New(Config{
		AppName:        "app",
		Model:          &scriptedLLM{name: "m"},
		SessionService: session.InMemoryService(),
		MemoryService:  memory.InMemoryService(),
		Instruction:    "custom instruction",
	})
	if err != nil {
		t.Fatalf("New with memory failed: %v", err)
	}
	if rt.AppName() != "app" {
		t.Errorf("AppName = %q", rt.AppName())
	}
}

var _ = agent.RunConfig{} // keep agent import used
