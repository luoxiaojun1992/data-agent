// Package adktools exposes the data-agent skills as ADK function tools.
// Session-scoped identity (user_id, role, kb_id) is injected from
// tool.Context.State() — the LLM never has to guess or supply it.
package adktools

import (
	"context"
	"fmt"
	"strings"

	reportpkg "github.com/luoxiaojun1992/data-agent/internal/logic/report"
	sqlpkg "github.com/luoxiaojun1992/data-agent/internal/logic/sql"
	statspkg "github.com/luoxiaojun1992/data-agent/internal/logic/stats"
	knowledgepkg "github.com/luoxiaojun1992/data-agent/internal/service/knowledge"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// Deps carries the service dependencies required by the tools.
type Deps struct {
	// KBService backs the knowledge_search tool. Required.
	KBService *knowledgepkg.Service
	// Memory backs the memory_search and memory_write tools.
	Memory memory.Service
	// MemoryWriter is an optional writer for agent-triggered memory_write.
	// If nil, memory_write returns an explanatory error.
	MemoryWriter MemoryWriter
	// AppName scopes memory searches.
	AppName string
}

// MemoryWriter writes content to long-term memory on agent request.
type MemoryWriter interface {
	WriteMemory(ctx context.Context, userID, content string) error
}

// stateString reads a string value from the tool session state.
func stateString(tc agent.ToolContext, key string) string {
	v, err := tc.State().Get(key)
	if err != nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// ---- sql_validate ----

// SQLValidateArgs are the arguments for the sql_validate tool.
type SQLValidateArgs struct {
	Query  string `json:"query" jsonschema:"SQL SELECT statement to validate"`
	Params []any  `json:"params,omitempty" jsonschema:"Parameterized query bind values"`
}

// SQLValidateResult is the outcome of SQL safety validation.
type SQLValidateResult struct {
	Status  string `json:"status"`
	Query   string `json:"query"`
	Message string `json:"message"`
	Reason  string `json:"reason,omitempty"`
}

func sqlValidate(tc agent.ToolContext, args SQLValidateArgs) (SQLValidateResult, error) {
	if strings.TrimSpace(args.Query) == "" {
		return SQLValidateResult{}, fmt.Errorf("sql_validate: missing required parameter 'query'")
	}
	result := sqlpkg.Validate(args.Query, args.Params)
	if !result.Allowed {
		return SQLValidateResult{}, fmt.Errorf("sql_validate: query rejected: %s", result.Reason)
	}
	return SQLValidateResult{
		Status:  "validated",
		Query:   args.Query,
		Message: "SQL query passed safety validation. Execute against your database.",
		Reason:  result.Reason,
	}, nil
}

// ---- stats_compute ----

// StatsComputeArgs are the arguments for the stats_compute tool.
type StatsComputeArgs struct {
	Method  string    `json:"method" jsonschema:"Analysis method: descriptive, linear_regression, time_series"`
	Values  []float64 `json:"values" jsonschema:"Numeric values for analysis"`
	Label   string    `json:"label,omitempty" jsonschema:"Optional label for descriptive stats"`
	XValues []float64 `json:"x_values,omitempty" jsonschema:"X values for linear regression"`
}

func statsCompute(tc agent.ToolContext, args StatsComputeArgs) (any, error) {
	if args.Method == "" {
		return nil, fmt.Errorf("stats_compute: missing required parameter 'method'")
	}
	if len(args.Values) == 0 {
		return nil, fmt.Errorf("stats_compute: 'values' must not be empty")
	}

	switch args.Method {
	case "descriptive":
		return statspkg.Descriptive(args.Values, args.Label), nil
	case "linear_regression":
		if len(args.XValues) == 0 {
			return nil, fmt.Errorf("stats_compute: 'x_values' required for linear_regression")
		}
		return statspkg.LinearRegression(args.XValues, args.Values), nil
	case "time_series":
		return statspkg.TimeSeriesDecompose(args.Values), nil
	default:
		return nil, fmt.Errorf("stats_compute: unknown method %q (valid: descriptive, linear_regression, time_series)", args.Method)
	}
}

// ---- knowledge_search ----

// KnowledgeSearchArgs are the arguments for the knowledge_search tool.
type KnowledgeSearchArgs struct {
	Query string `json:"query" jsonschema:"Search query string"`
	TopK  int    `json:"top_k,omitempty" jsonschema:"Maximum number of results (default 5, max 50)"`
}

// KnowledgeHit is one search result entry.
type KnowledgeHit struct {
	DocID   string  `json:"doc_id"`
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// KnowledgeSearchResult is the knowledge_search tool output.
type KnowledgeSearchResult struct {
	Query   string         `json:"query"`
	KBID    string         `json:"kb_id,omitempty"`
	Results []KnowledgeHit `json:"results"`
	Count   int            `json:"count"`
}

func knowledgeSearch(deps *Deps) functiontool.Func[KnowledgeSearchArgs, KnowledgeSearchResult] {
	return func(tc agent.ToolContext, args KnowledgeSearchArgs) (KnowledgeSearchResult, error) {
		if strings.TrimSpace(args.Query) == "" {
			return KnowledgeSearchResult{}, fmt.Errorf("knowledge_search: missing required parameter 'query'")
		}

		topK := args.TopK
		if topK <= 0 {
			topK = 5
		}
		if topK > 50 {
			topK = 50
		}

		// Identity comes from session state — never from LLM-supplied params.
		userID := stateString(tc, "user_id")
		role := stateString(tc, "role")
		kbID := stateString(tc, "kb_id")

		results, err := deps.KBService.Search(userID, args.Query, topK, role)
		if err != nil {
			return KnowledgeSearchResult{}, fmt.Errorf("knowledge_search: search failed: %w", err)
		}

		hits := make([]KnowledgeHit, 0, len(results))
		for _, r := range results {
			hits = append(hits, KnowledgeHit{
				DocID:   r.DocID,
				Title:   r.DocTitle,
				Content: truncateContent(r.Content, 500),
				Score:   r.Score,
			})
		}
		return KnowledgeSearchResult{Query: args.Query, KBID: kbID, Results: hits, Count: len(hits)}, nil
	}
}

// ---- save_report ----

// SaveReportArgs are the arguments for the save_report tool.
type SaveReportArgs struct {
	Title    string `json:"title" jsonschema:"Report title"`
	Content  string `json:"content" jsonschema:"Report content in markdown format"`
	Validate *bool  `json:"validate,omitempty" jsonschema:"Whether to validate mandatory sections (default true)"`
}

// SaveReportResult is the save_report tool output.
type SaveReportResult struct {
	Title            string   `json:"title"`
	Status           string   `json:"status"`
	Valid            bool     `json:"valid,omitempty"`
	DetectedSections []string `json:"detected_sections,omitempty"`
	MissingSections  []string `json:"missing_sections,omitempty"`
	Feedback         string   `json:"feedback,omitempty"`
}

func saveReport(tc agent.ToolContext, args SaveReportArgs) (SaveReportResult, error) {
	if strings.TrimSpace(args.Title) == "" {
		return SaveReportResult{}, fmt.Errorf("save_report: missing required parameter 'title'")
	}
	if strings.TrimSpace(args.Content) == "" {
		return SaveReportResult{}, fmt.Errorf("save_report: missing required parameter 'content'")
	}

	result := SaveReportResult{Title: args.Title, Status: "saved"}
	shouldValidate := args.Validate == nil || *args.Validate
	if shouldValidate {
		validation := reportpkg.Validate(args.Content)
		result.Valid = validation.Valid
		result.DetectedSections = validation.DetectedSections
		if !validation.Valid {
			result.MissingSections = validation.MissingSections
			result.Feedback = validation.Feedback
			result.Status = "validation_failed"
		}
	}
	return result, nil
}

// ---- memory_search ----

// MemorySearchArgs are the arguments for the memory_search tool.
type MemorySearchArgs struct {
	Query string `json:"query" jsonschema:"搜索关键词"`
	Limit int    `json:"limit,omitempty" jsonschema:"返回结果数（默认 5）"`
}

// MemorySearchResult is the memory_search tool output.
type MemorySearchResult struct {
	Memories []string `json:"memories"`
	Count    int      `json:"count"`
	Note     string   `json:"note,omitempty"`
}

func memorySearch(deps *Deps) functiontool.Func[MemorySearchArgs, MemorySearchResult] {
	return func(tc agent.ToolContext, args MemorySearchArgs) (MemorySearchResult, error) {
		if strings.TrimSpace(args.Query) == "" {
			return MemorySearchResult{}, fmt.Errorf("memory_search: missing required parameter 'query'")
		}
		if deps.Memory == nil {
			return MemorySearchResult{Memories: []string{}, Note: "memory service not configured"}, nil
		}

		resp, err := deps.Memory.SearchMemory(tc, &memory.SearchRequest{
			AppName: deps.AppName,
			UserID:  stateString(tc, "user_id"),
			Query:   args.Query,
		})
		if err != nil {
			return MemorySearchResult{}, fmt.Errorf("memory_search: %w", err)
		}
		return formatMemories(resp, args.Limit), nil
	}
}

// ---- memory_write ----

// MemoryWriteArgs are the arguments for the memory_write tool.
type MemoryWriteArgs struct {
	Content string `json:"content" jsonschema:"信息内容，要写入长期记忆的具体信息"`
}

// MemoryWriteResult is the memory_write tool output.
type MemoryWriteResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func memoryWrite(deps *Deps) functiontool.Func[MemoryWriteArgs, MemoryWriteResult] {
	return func(tc agent.ToolContext, args MemoryWriteArgs) (MemoryWriteResult, error) {
		if strings.TrimSpace(args.Content) == "" {
			return MemoryWriteResult{}, fmt.Errorf("memory_write: missing required parameter 'content'")
		}
		if deps.MemoryWriter == nil {
			return MemoryWriteResult{Status: "skipped", Message: "memory writer not configured"}, nil
		}
		userID := stateString(tc, "user_id")
		if err := deps.MemoryWriter.WriteMemory(tc, userID, args.Content); err != nil {
			return MemoryWriteResult{}, fmt.Errorf("memory_write: %w", err)
		}
		return MemoryWriteResult{Status: "written", Message: "memory stored"}, nil
	}
}

// formatMemories converts memory entries into the tool result, honoring the limit.
func formatMemories(resp *memory.SearchResponse, limit int) MemorySearchResult {
	if limit <= 0 {
		limit = 5
	}
	out := MemorySearchResult{Memories: []string{}}
	for i, m := range resp.Memories {
		if i >= limit {
			break
		}
		out.Memories = append(out.Memories, memoryEntryText(m))
	}
	out.Count = len(out.Memories)
	return out
}

// memoryEntryText concatenates the text parts of one memory entry.
func memoryEntryText(m memory.Entry) string {
	if m.Content == nil {
		return ""
	}
	var text strings.Builder
	for _, p := range m.Content.Parts {
		if p != nil {
			text.WriteString(p.Text)
		}
	}
	return text.String()
}

// ---- registry ----

// toolSpec describes one tool to register.
type toolSpec struct {
	name        string
	description string
	build       func() (tool.Tool, error)
}

func specs(deps *Deps) []toolSpec {
	out := []toolSpec{
		{
			name:        "sql_validate",
			description: "Validates SQL SELECT queries against safety rules before execution",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{Name: "sql_validate", Description: "Validates SQL SELECT queries against safety rules before execution"}, sqlValidate)
			},
		},
		{
			name:        "stats_compute",
			description: "Performs statistical analysis: descriptive stats, linear regression, time series decomposition",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{Name: "stats_compute", Description: "Performs statistical analysis: descriptive stats, linear regression, time series decomposition"}, statsCompute)
			},
		},
		{
			name:        "save_report",
			description: "Validates and saves analysis reports, ensuring mandatory sections are present",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{Name: "save_report", Description: "Validates and saves analysis reports, ensuring mandatory sections are present"}, saveReport)
			},
		},
		{
			name:        "memory_search",
			description: "Searches long-term memory for information from past conversations",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{Name: "memory_search", Description: "Searches long-term memory for information from past conversations"}, memorySearch(deps))
			},
		},
		{
			name:        "memory_write",
			description: "Writes a piece of information to long-term memory for later retrieval",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{Name: "memory_write", Description: "Writes a piece of information to long-term memory for later retrieval"}, memoryWrite(deps))
			},
		},
	}
	if deps.KBService != nil {
		out = append(out, toolSpec{
			name:        "knowledge_search",
			description: "Searches the knowledge base with full-text and semantic search capabilities",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{Name: "knowledge_search", Description: "Searches the knowledge base with full-text and semantic search capabilities"}, knowledgeSearch(deps))
			},
		})
	}
	return out
}

// All builds every ADK tool with the given dependencies.
// Tools whose required dependency is missing are skipped.
func All(deps *Deps) ([]tool.Tool, error) {
	specs := specs(deps)
	tools := make([]tool.Tool, 0, len(specs))
	for _, s := range specs {
		t, err := s.build()
		if err != nil {
			return nil, fmt.Errorf("build tool %q: %w", s.name, err)
		}
		tools = append(tools, t)
	}
	return tools, nil
}

// Names returns the tool names built by All (used by the skills listing API).
func Names(deps *Deps) ([]string, error) {
	tools, err := All(deps)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name())
	}
	return names, nil
}

func truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return strings.TrimSpace(content[:maxLen]) + "..."
}
