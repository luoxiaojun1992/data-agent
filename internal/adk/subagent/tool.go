package subagent

import (
	"fmt"
	"strings"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// InvokeSubAgentArgs are the arguments for the invoke_subagent tool.
type InvokeSubAgentArgs struct {
	// Task is the complete, self-contained task to delegate. The sub-agent has
	// no access to this conversation's history, so all necessary context must
	// be included here.
	Task string `json:"task" jsonschema:"Complete self-contained task description to delegate to a sub-agent. The sub-agent autonomously plans and executes multi-step work (data queries, statistical analysis, document/PPT generation, artifact saving) and returns a final result. Include ALL necessary context — the sub-agent has no access to this conversation's history."`
}

// InvokeSubAgentResult is the sub-agent's final output.
type InvokeSubAgentResult struct {
	Output string `json:"output"`
}

// NewTool builds the invoke_subagent tool (SPEC-071). The sub-agent uses the
// same model as the parent session and a trimmed tool set (which excludes
// invoke_subagent itself), preventing recursive delegation.
func NewTool(runner *Runner) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name: "invoke_subagent",
		Description: "Delegates a complex, multi-step task to an autonomous sub-agent and returns its final result. " +
			"Use this for work that requires multiple rounds of reasoning and tool calls (e.g. query data, run analysis, generate a report/PPT, save artifacts). " +
			"The sub-agent runs independently with the same capabilities as you (minus this delegation tool) and returns a single final text result.",
	}, invokeSubAgent(runner))
}

// invokeSubAgent is the tool handler: it reads identity from the tool context
// (never from the LLM), then launches the sub-agent via the Runner. The parent
// session ID and user ID come from the context; role/kb_id are propagated from
// the parent session state so the sub-agent's tools respect the same
// permissions.
func invokeSubAgent(runner *Runner) functiontool.Func[InvokeSubAgentArgs, InvokeSubAgentResult] {
	return func(tc agent.ToolContext, args InvokeSubAgentArgs) (InvokeSubAgentResult, error) {
		task := strings.TrimSpace(args.Task)
		if task == "" {
			return InvokeSubAgentResult{}, fmt.Errorf("invoke_subagent: task is required and must be non-empty")
		}
		parentSessionID := tc.SessionID()
		userID := tc.UserID()
		if parentSessionID == "" || userID == "" {
			return InvokeSubAgentResult{}, fmt.Errorf("invoke_subagent: missing session context (session_id/user_id)")
		}

		// Identity + parent-session binding for the sub-agent's tools. The
		// parent session ID keeps file/artifact side effects in the parent's
		// context; the sub-agent's history is a separate (destroyed) session.
		state := map[string]any{
			"user_id":    userID,
			"session_id": parentSessionID,
		}
		for _, key := range []string{"role", "kb_id"} {
			if v, err := tc.State().Get(key); err == nil && v != nil {
				state[key] = v
			}
		}

		output, err := runner.Run(tc, parentSessionID, userID, task, state)
		if err != nil {
			return InvokeSubAgentResult{}, err
		}
		return InvokeSubAgentResult{Output: output}, nil
	}
}
