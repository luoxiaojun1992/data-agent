// Package adktools exposes the data-agent skills as ADK function tools.
// Session-scoped identity (user_id, role, kb_id) is injected from
// tool.Context.State() — the LLM never has to guess or supply it.
package adktools

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	fsops "github.com/luoxiaojun1992/data-agent/internal/logic/fsops"
	pptxpkg "github.com/luoxiaojun1992/data-agent/internal/logic/pptx"
	sqlpkg "github.com/luoxiaojun1992/data-agent/internal/logic/sql"
	statspkg "github.com/luoxiaojun1992/data-agent/internal/logic/stats"
	websearchpkg "github.com/luoxiaojun1992/data-agent/internal/logic/websearch"
	webfetchpkg "github.com/luoxiaojun1992/data-agent/internal/logic/webfetch"
	artifact_svc "github.com/luoxiaojun1992/data-agent/internal/service/artifact"
	apicollectionsvc "github.com/luoxiaojun1992/data-agent/internal/service/apicollection"
	chatsvc "github.com/luoxiaojun1992/data-agent/internal/service/chat"
	skillsvc "github.com/luoxiaojun1992/data-agent/internal/service/skill"
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
	// SkillConfig backs sql_executor config reads.
	SkillConfig *skillsvc.ConfigService
	// Memory backs the memory_search and memory_write tools.
	Memory memory.Service
	// MemoryWriter is an optional writer for agent-triggered memory_write.
	// If nil, memory_write returns an explanatory error.
	MemoryWriter MemoryWriter
	// AppName scopes memory searches.
	AppName string
	// Tasks backs the save_task_result tool (task-mode runs only).
	// If nil, save_task_result returns an explanatory error.
	Tasks domaintask.TaskRunService
	// SessionSvc resolves session workspace paths.
	SessionSvc domainchat.SessionService
	// Artifacts backs the save_artifact tool (create record + upload).
	Artifacts artifact_svc.Service
	// APICollections backs the external_api_* tools for proxying approved API calls.
	APICollections *apicollectionsvc.Service
}

// MemoryWriter writes content to long-term memory on agent request.
type MemoryWriter interface {
	WriteMemory(ctx context.Context, userID, sessionID, content string) error
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

// ---- sql_executor ----

// SQLExecutorArgs are the arguments for the sql_executor tool.
type SQLExecutorArgs struct {
	Query  string `json:"query" jsonschema:"SQL SELECT statement to validate and execute"`
	Params []any  `json:"params,omitempty" jsonschema:"Parameterized query bind values"`
}

// SQLExecutorResult is the outcome of SQL execution.
type SQLExecutorResult sqlpkg.ExecResult

func sqlExecutor(deps *Deps) functiontool.Func[SQLExecutorArgs, SQLExecutorResult] {
	return func(tc agent.ToolContext, args SQLExecutorArgs) (SQLExecutorResult, error) {
		if strings.TrimSpace(args.Query) == "" {
			return SQLExecutorResult{}, fmt.Errorf("sql_executor: missing required parameter 'query'")
		}

		// Read skill config for MySQL connection
		var execCfg sqlpkg.ExecConfig
		if deps.SkillConfig != nil {
			_ = deps.SkillConfig.GetConfig(tc, "sql_executor", &execCfg)
		}
		if execCfg.DSN == "" {
			return SQLExecutorResult{}, fmt.Errorf("sql_executor: MySQL not configured — set DSN in admin skill config")
		}

		result, err := sqlpkg.Execute(execCfg, args.Query, args.Params)
		return SQLExecutorResult(result), err
	}
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

		// Identity and permission flags come from session state — never from LLM params.
		// Force-bound: user_id, role, is_system_admin determine visibility filter.
		userID := stateString(tc, "user_id")
		role := stateString(tc, "role")
		kbID := stateString(tc, "kb_id")
		isSystemAdmin := role == "system_admin"

		results, err := deps.KBService.Search(userID, args.Query, topK, isSystemAdmin)
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

// ---- memory_search ----

// MemorySearchArgs are the arguments for the memory_search tool.
type MemorySearchArgs struct {
	Query string `json:"query" jsonschema:"搜索 query（用 1-2 个短小词，正则匹配，长句可能匹配不到）"`
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
		sessionID := stateString(tc, "session_id")
		if err := deps.MemoryWriter.WriteMemory(tc, userID, sessionID, args.Content); err != nil {
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

// ---- save_task_result ----

// SaveTaskResultArgs are the arguments for the save_task_result tool.
type SaveTaskResultArgs struct {
	// Content is the task's final result text the user will see. Required
	// and must be non-empty after trimming whitespace.
	Content string `json:"content" jsonschema:"Task result content; must be non-empty"`
	// Status is the result status: success (default) or failed. Failed still
	// persists the content as the user-visible result but lets the orchestrator
	// mark the task as failed.
	Status string `json:"status,omitempty" jsonschema:"Result status: 'success' (default) or 'failed'"`
}

// SaveTaskResultResult is the save_task_result tool output.
type SaveTaskResultResult struct {
	TaskID  string `json:"task_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func saveTaskResult(deps *Deps) functiontool.Func[SaveTaskResultArgs, SaveTaskResultResult] {
	return func(tc agent.ToolContext, args SaveTaskResultArgs) (SaveTaskResultResult, error) {
		if deps.Tasks == nil {
			return SaveTaskResultResult{}, fmt.Errorf("save_task_result: task service not configured")
		}
		// run_id is injected from session state, not from the LLM — the
		// model must not be able to save results for an arbitrary run.
		runID := stateString(tc, "run_id")
		if runID == "" {
			return SaveTaskResultResult{}, fmt.Errorf("save_task_result: not running in a task context (no run_id in session state)")
		}
		if strings.TrimSpace(args.Content) == "" {
			return SaveTaskResultResult{}, fmt.Errorf("save_task_result: content is required and must be non-empty")
		}
		status := strings.ToLower(strings.TrimSpace(args.Status))
		if status == "" {
			status = "success"
		}
		if status != "success" && status != "failed" {
			return SaveTaskResultResult{}, fmt.Errorf("save_task_result: invalid status %q (allowed: success, failed)", args.Status)
		}
		result := map[string]interface{}{
			"content": args.Content,
			"status":  status,
		}
		if status == "failed" {
			if err := deps.Tasks.UpdateRunError(runID, args.Content); err != nil {
				return SaveTaskResultResult{}, fmt.Errorf("save_task_result: update run error: %w", err)
			}
		} else {
			if err := deps.Tasks.UpdateRunResult(runID, result); err != nil {
				return SaveTaskResultResult{}, fmt.Errorf("save_task_result: update run result: %w", err)
			}
		}
		return SaveTaskResultResult{
			TaskID:  runID,
			Status:  status,
			Message: "task result saved",
		}, nil
	}
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
			name:        "sql_executor",
			description: "Validates and executes safe SQL SELECT queries against the configured MySQL database. Validation blocks DML/DDL statements.",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{Name: "sql_executor", Description: "Validates and executes safe SQL SELECT queries against the configured MySQL database. Validation blocks DML/DDL statements."}, sqlExecutor(deps))
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
			name:        "memory_search",
			description: "Searches long-term memory for information from past conversations. Pass a SHORT query (the 'query' parameter) of 1-2 words like '销售', '部署', '模型' — the search uses regex matching, so long phrases will NOT match.",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{Name: "memory_search", Description: "Searches long-term memory for information from past conversations. Pass a SHORT query (the 'query' parameter) of 1-2 words like '销售', '部署', '模型' — the search uses regex matching, so long phrases will NOT match."}, memorySearch(deps))
			},
		},
		{
			name:        "web_search",
			description: "Searches the web via Bing/Baidu APIs to find real-time information. Requires bing_api_key or baidu_api_key in skill config (set via Admin panel). If no API key is configured, returns an error explaining how to enable it.",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "web_search",
					Description: "Searches the web via Bing/Baidu APIs to find real-time information. Requires bing_api_key or baidu_api_key in skill config (set via Admin panel). If no API key is configured, returns an error explaining how to enable it.",
				}, webSearch(deps))
			},
		},
		{
			name:        "web_fetch",
			description: "Fetches a single web page by URL using HTTP GET and returns its extracted plain text (title + body). Only GET is supported. Use this to read the content of a specific page or URL.",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "web_fetch",
					Description: "Fetches a single web page by URL using HTTP GET and returns its extracted plain text (title + body). Only GET is supported. Use this to read the content of a specific page or URL.",
				}, webFetch(deps))
			},
		},
		{
			name:        "memory_write",
			description: "Writes a piece of information to long-term memory for later retrieval",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{Name: "memory_write", Description: "Writes a piece of information to long-term memory for later retrieval"}, memoryWrite(deps))
			},
		},
		{
			name:        "skill_search",
			description: "Searches enabled skills by matching the skill DESCRIPTION only (not name or display name). Pass a SHORT query (the 'query' parameter) of 1-2 words — long phrases will NOT match well. Returns a lightweight list of matching skills (name, display_name, description) without detailed config. Use this to discover what skills are available. You can also look up this skill itself by searching 'search' or 'skill'.",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{Name: "skill_search", Description: "Searches enabled skills by matching the skill DESCRIPTION only (not name or display name). Pass a SHORT query (the 'query' parameter) of 1-2 words — long phrases will NOT match well. Returns a lightweight list of matching skills (name, display_name, description) without detailed config. Use this to discover what skills are available. You can also look up this skill itself by searching 'search' or 'skill'."}, skillSearch(deps))
			},
		},
		{
			name:        "skill_detail",
			description: "Returns the full detailed configuration of a single ENABLED skill by its exact NAME (not display name), including enabled status and config_json. Disabled skills are not returned. Use the name returned by skill_search. You can also look up this skill itself with name 'skill_detail'.",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{Name: "skill_detail", Description: "Returns the full detailed configuration of a single ENABLED skill by its exact NAME (not display name), including enabled status and config_json. Disabled skills are not returned. Use the name returned by skill_search. You can also look up this skill itself with name 'skill_detail'."}, skillDetail(deps))
			},
		},
	}
	if deps.APICollections != nil {
		out = append(out,
			toolSpec{
				name:        "external_api_search",
				description: "Searches approved API collections by description using regex matching. Pass a SHORT query (the 'query' parameter) of 1-2 words like 'user', 'order', 'weather' — long phrases will NOT match. Returns up to 3 top results.",
				build: func() (tool.Tool, error) {
					return functiontool.New(functiontool.Config{Name: "external_api_search", Description: "Searches approved API collections by description using regex matching. Pass a SHORT query (the 'query' parameter) of 1-2 words like 'user', 'order', 'weather' — long phrases will NOT match. Returns up to 3 top results."}, externalAPISearch(deps))
				},
			},
			toolSpec{
				name:        "external_api_summary",
				description: "Lists all API methods in an approved collection given its ID",
				build: func() (tool.Tool, error) {
					return functiontool.New(functiontool.Config{Name: "external_api_summary", Description: "Lists all API methods in an approved collection given its ID"}, externalAPISummary(deps))
				},
			},
			toolSpec{
				name:        "external_api_method",
				description: "Shows detailed parameter information for a specific API method",
				build: func() (tool.Tool, error) {
					return functiontool.New(functiontool.Config{Name: "external_api_method", Description: "Shows detailed parameter information for a specific API method"}, externalAPIMethod(deps))
				},
			},
			toolSpec{
				name:        "external_api_call",
				description: "Calls an approved external API endpoint with given parameters and returns the response",
				build: func() (tool.Tool, error) {
					return functiontool.New(functiontool.Config{Name: "external_api_call", Description: "Calls an approved external API endpoint with given parameters and returns the response"}, externalAPICall(deps))
				},
			},
		)
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
	if deps.Tasks != nil {
		out = append(out, toolSpec{
			name:        "save_task_result",
			description: "Persists the final result of an async/scheduled task. The task_id is read from the session state (not supplied by the LLM). The content argument is required and must be non-empty; the task is marked completed on success or failed if status=\"failed\" is set. Without this call the task has no result and the system retries once.",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{Name: "save_task_result", Description: "Persists the final result of an async/scheduled task. The task_id is read from the session state (not supplied by the LLM). The content argument is required and must be non-empty; the task is marked completed on success or failed if status=\"failed\" is set. Without this call the task has no result and the system retries once."}, saveTaskResult(deps))
			},
		})
	}
	if deps.SessionSvc != nil {
		out = append(out, toolSpec{
			name:        "pptx_generator",
			description: "Generates a .pptx PowerPoint file from markdown content and saves it in the session workspace. The content should be well-structured markdown with # slide titles and - bullet points. Returns the relative file path (e.g. output.pptx).",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{Name: "pptx_generator", Description: "Generates a .pptx PowerPoint file from markdown content and saves it in the session workspace. The content should be well-structured markdown with # slide titles and - bullet points. Returns the relative file path (e.g. output.pptx)."}, pptxGenerator(deps))
			},
		})
		out = append(out, toolSpec{
			name:        "save_artifact",
			description: "Saves a file or directory from the session workspace as a persistent artifact. Packages the file/directory into a zip, uploads it, and creates an artifact record associated with the current session and user. Returns the artifact ID and download URL.",
			build: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{Name: "save_artifact", Description: "Saves a file or directory from the session workspace as a persistent artifact. Packages the file/directory into a zip, uploads it, and creates an artifact record associated with the current session and user. Returns the artifact ID and download URL."}, saveArtifact(deps))
			},
		})
		out = append(out,
			toolSpec{
				name:        "file_write",
				description: "Writes (or overwrites) a text file inside the current session workspace. 'path' is relative to the session workspace and its parent directory must already exist (this tool does NOT create missing directories — call dir_create first if needed). 'content' is the full text to write. All file operations are strictly scoped to the session workspace; absolute paths and '..' are rejected.",
				build: func() (tool.Tool, error) {
					return functiontool.New(functiontool.Config{Name: "file_write", Description: "Writes (or overwrites) a text file inside the current session workspace. 'path' is relative to the session workspace and its parent directory must already exist (this tool does NOT create missing directories — call dir_create first if needed). 'content' is the full text to write. All file operations are strictly scoped to the session workspace; absolute paths and '..' are rejected."}, fileWrite(deps))
				},
			},
			toolSpec{
				name:        "dir_create",
				description: "Creates a directory inside the current session workspace. 'path' is relative to the session workspace; missing parent directories are created recursively.",
				build: func() (tool.Tool, error) {
					return functiontool.New(functiontool.Config{Name: "dir_create", Description: "Creates a directory inside the current session workspace. 'path' is relative to the session workspace; missing parent directories are created recursively."}, dirCreate(deps))
				},
			},
			toolSpec{
				name:        "file_delete",
				description: "Deletes a single file inside the current session workspace. 'path' is relative to the session workspace. Directories are refused (use dir_delete for those).",
				build: func() (tool.Tool, error) {
					return functiontool.New(functiontool.Config{Name: "file_delete", Description: "Deletes a single file inside the current session workspace. 'path' is relative to the session workspace. Directories are refused (use dir_delete for those)."}, fileDelete(deps))
				},
			},
			toolSpec{
				name:        "dir_delete",
				description: "Recursively deletes a directory inside the current session workspace, including all its subdirectories and files. 'path' is relative to the session workspace. The workspace root cannot be deleted.",
				build: func() (tool.Tool, error) {
					return functiontool.New(functiontool.Config{Name: "dir_delete", Description: "Recursively deletes a directory inside the current session workspace, including all its subdirectories and files. 'path' is relative to the session workspace. The workspace root cannot be deleted."}, dirDelete(deps))
				},
			},
			toolSpec{
				name:        "file_read",
				description: "Reads a line range from a text file inside the current session workspace. 'path' is relative to the session workspace. start_line/end_line are 1-based and inclusive; if omitted, the first 10 lines are returned. Never returns the entire file in one call — use total_lines to page through larger files.",
				build: func() (tool.Tool, error) {
					return functiontool.New(functiontool.Config{Name: "file_read", Description: "Reads a line range from a text file inside the current session workspace. 'path' is relative to the session workspace. start_line/end_line are 1-based and inclusive; if omitted, the first 10 lines are returned. Never returns the entire file in one call — use total_lines to page through larger files."}, fileRead(deps))
				},
			},
			toolSpec{
				name:        "dir_list",
				description: "Lists the immediate children (files and subdirectories) of a directory inside the current session workspace, non-recursively. 'path' is relative to the session workspace; omit it to list the workspace root. Use this to explore what files/directories exist before reading or writing.",
				build: func() (tool.Tool, error) {
					return functiontool.New(functiontool.Config{Name: "dir_list", Description: "Lists the immediate children (files and subdirectories) of a directory inside the current session workspace, non-recursively. 'path' is relative to the session workspace; omit it to list the workspace root. Use this to explore what files/directories exist before reading or writing."}, dirList(deps))
				},
			},
		)
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

// ---- web_search ----

// WebSearchArgs are the arguments for the web_search tool.
type WebSearchArgs struct {
	Query string `json:"query" description:"The search query to look up on the web"`
}

// webSearch performs a web search via configured engines (Bing/Baidu).
// API keys are read from skill config; empty key → engine skipped.
func webSearch(deps *Deps) functiontool.Func[WebSearchArgs, websearchpkg.SearchResult] {
	return func(ctx agent.ToolContext, args WebSearchArgs) (websearchpkg.SearchResult, error) {
		if args.Query == "" {
			return websearchpkg.SearchResult{Error: "query is required"}, nil
		}
		var cfg websearchpkg.Config
		if deps.SkillConfig != nil {
			_ = deps.SkillConfig.GetConfig(context.Background(), "web_search", &cfg)
		}
		result, err := websearchpkg.Search(context.Background(), args.Query, cfg)
		if err != nil {
			return websearchpkg.SearchResult{Error: err.Error()}, nil
		}
		return *result, nil
	}
}

// ---- web_fetch ----

// WebFetchArgs are the arguments for the web_fetch tool.
type WebFetchArgs struct {
	URL string `json:"url" description:"The full web page URL to fetch (must start with http:// or https://)"`
}

// webFetch fetches a single page via HTTP GET and returns extracted text.
func webFetch(deps *Deps) functiontool.Func[WebFetchArgs, webfetchpkg.Result] {
	return func(ctx agent.ToolContext, args WebFetchArgs) (webfetchpkg.Result, error) {
		var cfg webfetchpkg.Config
		if deps.SkillConfig != nil {
			_ = deps.SkillConfig.GetConfig(context.Background(), "web_fetch", &cfg)
		}
		result, err := webfetchpkg.Fetch(context.Background(), args.URL, cfg)
		if err != nil {
			return webfetchpkg.Result{Error: err.Error()}, nil
		}
		return *result, nil
	}
}

func truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return strings.TrimSpace(content[:maxLen]) + "..."
}

// ---- skill_search ----

// SkillSearchArgs are the arguments for the skill_search tool.
type SkillSearchArgs struct {
	Query string `json:"query" jsonschema:"Short query (1-2 words) to match against skill descriptions, e.g. '文件', '搜索', '模型'. Long phrases will NOT match well."`
	TopN  int    `json:"top_n,omitempty" jsonschema:"Maximum number of results (default 5, max 20)."`
}

// SkillSearchItem is a single lightweight skill match (no detailed config).
type SkillSearchItem struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
}

// SkillSearchResult is the outcome of skill_search.
type SkillSearchResult struct {
	Items []SkillSearchItem `json:"items"`
}

func skillSearch(deps *Deps) functiontool.Func[SkillSearchArgs, SkillSearchResult] {
	return func(ctx agent.ToolContext, args SkillSearchArgs) (SkillSearchResult, error) {
		if deps.SkillConfig == nil {
			return SkillSearchResult{}, fmt.Errorf("skill_search: skill config service not available")
		}
		query := strings.TrimSpace(args.Query)
		if query == "" {
			return SkillSearchResult{Items: []SkillSearchItem{}}, nil
		}
		topN := args.TopN
		if topN <= 0 {
			topN = 5
		}
		if topN > 20 {
			topN = 20
		}
		results, err := deps.SkillConfig.SearchByDescription(context.Background(), query, topN)
		if err != nil {
			return SkillSearchResult{}, fmt.Errorf("skill_search: %w", err)
		}
		items := make([]SkillSearchItem, 0, len(results))
		for _, sk := range results {
			items = append(items, SkillSearchItem{
				Name:        sk.Name,
				DisplayName: sk.DisplayName,
				Description: sk.Description,
			})
		}
		return SkillSearchResult{Items: items}, nil
	}
}

// ---- skill_detail ----

// SkillDetailArgs are the arguments for the skill_detail tool.
type SkillDetailArgs struct {
	Name string `json:"name" jsonschema:"Exact skill name (not display name), e.g. 'sql_executor' or 'skill_detail'."`
}

// SkillDetailResult is the full detail of one skill.
type SkillDetailResult struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	ConfigJSON  string `json:"config_json"`
}

func skillDetail(deps *Deps) functiontool.Func[SkillDetailArgs, SkillDetailResult] {
	return func(ctx agent.ToolContext, args SkillDetailArgs) (SkillDetailResult, error) {
		if deps.SkillConfig == nil {
			return SkillDetailResult{}, fmt.Errorf("skill_detail: skill config service not available")
		}
		name := strings.TrimSpace(args.Name)
		if name == "" {
			return SkillDetailResult{}, fmt.Errorf("skill_detail: 'name' must not be empty")
		}
		cfg, err := deps.SkillConfig.Get(context.Background(), name)
		if err != nil {
			return SkillDetailResult{}, fmt.Errorf("skill_detail: %w", err)
		}
		if !cfg.Enabled {
			return SkillDetailResult{}, fmt.Errorf("skill %q is disabled", name)
		}
		return SkillDetailResult{
			Name:        cfg.Name,
			DisplayName: cfg.DisplayName,
			Description: cfg.Description,
			Enabled:     cfg.Enabled,
			ConfigJSON:  cfg.ConfigJSON,
		}, nil
	}
}

// ---- pptx_generator ----

// PPTXGeneratorArgs are the arguments for the pptx_generator tool.
type PPTXGeneratorArgs struct {
	Content  string `json:"content" jsonschema:"Markdown content for the presentation. Use # for slide titles, ## for subtitles, - for bullet points."`
	FileName string `json:"file_name,omitempty" jsonschema:"Output file name (default: presentation.pptx)"`
}

// PPTXGeneratorResult is the outcome of pptx generation.
type PPTXGeneratorResult struct {
	Path string `json:"path"` // relative path within session workspace
}

func pptxGenerator(deps *Deps) functiontool.Func[PPTXGeneratorArgs, PPTXGeneratorResult] {
	return func(tc agent.ToolContext, args PPTXGeneratorArgs) (PPTXGeneratorResult, error) {
		if strings.TrimSpace(args.Content) == "" {
			return PPTXGeneratorResult{}, fmt.Errorf("pptx_generator: 'content' must not be empty")
		}
		fileName := args.FileName
		if fileName == "" {
			fileName = "presentation.pptx"
		}
		if !strings.HasSuffix(strings.ToLower(fileName), ".pptx") {
			fileName += ".pptx"
		}

		sessionID := stateString(tc, "session_id")
		ws := chatsvc.SessionWorkspace(sessionID)
		fullPath := filepath.Join(ws, fileName)

		if err := pptxpkg.Generate(args.Content, fullPath); err != nil {
			return PPTXGeneratorResult{}, fmt.Errorf("pptx_generator: %w", err)
		}
		return PPTXGeneratorResult{Path: fileName}, nil
	}
}

// ---- save_artifact ----

// SaveArtifactArgs are the arguments for the save_artifact tool.
type SaveArtifactArgs struct {
	Path string `json:"path" jsonschema:"Relative path to the file or directory within the session workspace"`
}

// SaveArtifactResult is the outcome of save_artifact.
type SaveArtifactResult struct {
	ArtifactID  string `json:"artifact_id"`
	DownloadURL string `json:"download_url"`
	Name        string `json:"name"`
}

func saveArtifact(deps *Deps) functiontool.Func[SaveArtifactArgs, SaveArtifactResult] {
	return func(tc agent.ToolContext, args SaveArtifactArgs) (SaveArtifactResult, error) {
		if strings.TrimSpace(args.Path) == "" {
			return SaveArtifactResult{}, fmt.Errorf("save_artifact: 'path' must not be empty")
		}

		userID := stateString(tc, "user_id")
		sessionID := stateString(tc, "session_id")
		if userID == "" || sessionID == "" {
			return SaveArtifactResult{}, fmt.Errorf("save_artifact: session/user context not available")
		}

		ws := chatsvc.SessionWorkspace(sessionID)
		srcPath := filepath.Join(ws, filepath.Clean(args.Path))
		if !strings.HasPrefix(srcPath, ws) {
			return SaveArtifactResult{}, fmt.Errorf("save_artifact: path traversal denied")
		}

		info, err := os.Stat(srcPath)
		if err != nil {
			return SaveArtifactResult{}, fmt.Errorf("save_artifact: %w", err)
		}

		// Zip the file/directory into a temp buffer.
		var buf bytes.Buffer
		zipName := filepath.Base(args.Path)
		if info.IsDir() {
			zipName += ".zip"
		}
		if err := zipToWriter(srcPath, info, &buf); err != nil {
			return SaveArtifactResult{}, fmt.Errorf("save_artifact: zip: %w", err)
		}

		// Upload via artifact module (handles storage + DB record).
		a, err := deps.Artifacts.Upload(userID, sessionID, "", zipName, "application/zip", &buf, true)
		if err != nil {
			return SaveArtifactResult{}, fmt.Errorf("save_artifact: upload: %w", err)
		}

		return SaveArtifactResult{
			ArtifactID:  a.ID,
			DownloadURL: fmt.Sprintf("/api/v1/artifacts/%s/download", a.ID),
			Name:        a.Name,
		}, nil
	}
}

// ---- file_write ----

// FileWriteArgs are the arguments for the file_write tool.
type FileWriteArgs struct {
	Path    string `json:"path" jsonschema:"Relative path to the file within the session workspace. The parent directory must already exist (this tool does NOT create directories)."`
	Content string `json:"content" jsonschema:"Full text content to write to the file."`
}

// FileWriteResult is the outcome of file_write.
type FileWriteResult struct {
	Path string `json:"path"`
	Size int    `json:"size"`
}

func fileWrite(deps *Deps) functiontool.Func[FileWriteArgs, FileWriteResult] {
	return func(tc agent.ToolContext, args FileWriteArgs) (FileWriteResult, error) {
		if strings.TrimSpace(args.Path) == "" {
			return FileWriteResult{}, fmt.Errorf("file_write: 'path' must not be empty")
		}
		ws, err := sessionWorkspace(tc)
		if err != nil {
			return FileWriteResult{}, err
		}
		if err := fsops.WriteFile(ws, args.Path, args.Content); err != nil {
			return FileWriteResult{}, fmt.Errorf("file_write: %w", err)
		}
		return FileWriteResult{Path: args.Path, Size: len(args.Content)}, nil
	}
}

// ---- dir_create ----

// DirCreateArgs are the arguments for the dir_create tool.
type DirCreateArgs struct {
	Path string `json:"path" jsonschema:"Relative path to the directory to create within the session workspace. Missing parent directories are created recursively."`
}

// DirCreateResult is the outcome of dir_create.
type DirCreateResult struct {
	Path string `json:"path"`
}

func dirCreate(deps *Deps) functiontool.Func[DirCreateArgs, DirCreateResult] {
	return func(tc agent.ToolContext, args DirCreateArgs) (DirCreateResult, error) {
		if strings.TrimSpace(args.Path) == "" {
			return DirCreateResult{}, fmt.Errorf("dir_create: 'path' must not be empty")
		}
		ws, err := sessionWorkspace(tc)
		if err != nil {
			return DirCreateResult{}, err
		}
		if err := fsops.MkdirAll(ws, args.Path); err != nil {
			return DirCreateResult{}, fmt.Errorf("dir_create: %w", err)
		}
		return DirCreateResult{Path: args.Path}, nil
	}
}

// ---- file_delete ----

// FileDeleteArgs are the arguments for the file_delete tool.
type FileDeleteArgs struct {
	Path string `json:"path" jsonschema:"Relative path to the file to delete within the session workspace."`
}

// FileDeleteResult is the outcome of file_delete.
type FileDeleteResult struct {
	Path string `json:"path"`
}

func fileDelete(deps *Deps) functiontool.Func[FileDeleteArgs, FileDeleteResult] {
	return func(tc agent.ToolContext, args FileDeleteArgs) (FileDeleteResult, error) {
		if strings.TrimSpace(args.Path) == "" {
			return FileDeleteResult{}, fmt.Errorf("file_delete: 'path' must not be empty")
		}
		ws, err := sessionWorkspace(tc)
		if err != nil {
			return FileDeleteResult{}, err
		}
		if err := fsops.RemoveFile(ws, args.Path); err != nil {
			return FileDeleteResult{}, fmt.Errorf("file_delete: %w", err)
		}
		return FileDeleteResult{Path: args.Path}, nil
	}
}

// ---- dir_delete ----

// DirDeleteArgs are the arguments for the dir_delete tool.
type DirDeleteArgs struct {
	Path string `json:"path" jsonschema:"Relative path to the directory to delete within the session workspace. The directory and ALL its children are removed recursively."`
}

// DirDeleteResult is the outcome of dir_delete.
type DirDeleteResult struct {
	Path string `json:"path"`
}

func dirDelete(deps *Deps) functiontool.Func[DirDeleteArgs, DirDeleteResult] {
	return func(tc agent.ToolContext, args DirDeleteArgs) (DirDeleteResult, error) {
		if strings.TrimSpace(args.Path) == "" {
			return DirDeleteResult{}, fmt.Errorf("dir_delete: 'path' must not be empty")
		}
		ws, err := sessionWorkspace(tc)
		if err != nil {
			return DirDeleteResult{}, err
		}
		if err := fsops.RemoveDir(ws, args.Path); err != nil {
			return DirDeleteResult{}, fmt.Errorf("dir_delete: %w", err)
		}
		return DirDeleteResult{Path: args.Path}, nil
	}
}

// ---- file_read ----

// FileReadArgs are the arguments for the file_read tool.
type FileReadArgs struct {
	Path      string `json:"path" jsonschema:"Relative path to the file to read within the session workspace."`
	StartLine int    `json:"start_line,omitempty" jsonschema:"1-based starting line number (default 1)."`
	EndLine   int    `json:"end_line,omitempty" jsonschema:"1-based ending line number, inclusive. Defaults to start_line+9 (a 10-line window)."`
}

// FileReadResult is the outcome of file_read.
type FileReadResult struct {
	Path       string `json:"path"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	TotalLines int    `json:"total_lines"`
	Content    string `json:"content"`
}

func fileRead(deps *Deps) functiontool.Func[FileReadArgs, FileReadResult] {
	return func(tc agent.ToolContext, args FileReadArgs) (FileReadResult, error) {
		if strings.TrimSpace(args.Path) == "" {
			return FileReadResult{}, fmt.Errorf("file_read: 'path' must not be empty")
		}
		ws, err := sessionWorkspace(tc)
		if err != nil {
			return FileReadResult{}, err
		}
		res, err := fsops.ReadFile(ws, args.Path, args.StartLine, args.EndLine)
		if err != nil {
			return FileReadResult{}, fmt.Errorf("file_read: %w", err)
		}
		return FileReadResult(res), nil
	}
}

// ---- dir_list ----

// DirListArgs are the arguments for the dir_list tool.
type DirListArgs struct {
	Path string `json:"path,omitempty" jsonschema:"Relative path to the directory to list within the session workspace. Omit or use empty to list the workspace root. Only the immediate children are returned (non-recursive)."`
}

// DirListResult is the outcome of dir_list.
type DirListResult struct {
	Path    string        `json:"path"`
	Entries []fsops.Entry `json:"entries"`
}

func dirList(deps *Deps) functiontool.Func[DirListArgs, DirListResult] {
	return func(tc agent.ToolContext, args DirListArgs) (DirListResult, error) {
		ws, err := sessionWorkspace(tc)
		if err != nil {
			return DirListResult{}, err
		}
		res, err := fsops.ListDir(ws, args.Path)
		if err != nil {
			return DirListResult{}, fmt.Errorf("dir_list: %w", err)
		}
		return DirListResult{Path: res.Path, Entries: res.Entries}, nil
	}
}

// sessionWorkspace resolves the current session's workspace root from the
// tool session state. The session_id is injected by the runtime, never
// supplied by the LLM.
func sessionWorkspace(tc agent.ToolContext) (string, error) {
	sessionID := stateString(tc, "session_id")
	if sessionID == "" {
		return "", fmt.Errorf("session context not available")
	}
	return chatsvc.SessionWorkspace(sessionID), nil
}

// zipToWriter creates a zip archive in w from srcPath (file or directory).
func zipToWriter(srcPath string, info os.FileInfo, w io.Writer) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	if info.IsDir() {
		return filepath.Walk(srcPath, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(filepath.Dir(srcPath), path)
			if err != nil {
				return err
			}
			if fi.IsDir() {
				if rel != "." {
					_, err = zw.Create(rel + "/")
				}
				return err
			}
			f, err := zw.Create(rel)
			if err != nil {
				return err
			}
			src, err := os.Open(path)
			if err != nil {
				return err
			}
			defer src.Close()
			_, err = io.Copy(f, src)
			return err
		})
	}
	// Single file.
	f, err := zw.Create(info.Name())
	if err != nil {
		return err
	}
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	_, err = io.Copy(f, src)
	return err
}

// ---- external_api_* ----

type ExternalAPISearchArgs struct {
	Query string `json:"query" description:"Short query (1-2 words) to match against approved API collection descriptions"`
}
type ExternalAPISearchResult struct {
	Collections []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		APICount    int    `json:"api_count"`
	} `json:"collections"`
}

func externalAPISearch(deps *Deps) functiontool.Func[ExternalAPISearchArgs, ExternalAPISearchResult] {
	return func(ctx agent.ToolContext, args ExternalAPISearchArgs) (ExternalAPISearchResult, error) {
		if args.Query == "" {
			return ExternalAPISearchResult{}, nil
		}
		items, err := deps.APICollections.SearchApproved(context.Background(), args.Query, 3)
		if err != nil {
			return ExternalAPISearchResult{}, err
		}
		var out ExternalAPISearchResult
		for _, c := range items {
			out.Collections = append(out.Collections, struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				Description string `json:"description"`
				APICount    int    `json:"api_count"`
			}{c.ID, c.Name, c.Description, c.APICount})
		}
		return out, nil
	}
}

type ExternalAPISummaryArgs struct {
	CollectionID string `json:"collection_id" description:"The API collection ID to list methods for"`
}
type ExternalAPISummaryResult struct {
	Name  string              `json:"name"`
	Paths []externalPathEntry `json:"paths"`
}
type externalPathEntry struct {
	Path    string `json:"path"`
	Method  string `json:"method"`
	Summary string `json:"summary"`
}

func externalAPISummary(deps *Deps) functiontool.Func[ExternalAPISummaryArgs, ExternalAPISummaryResult] {
	return func(ctx agent.ToolContext, args ExternalAPISummaryArgs) (ExternalAPISummaryResult, error) {
		summary, err := deps.APICollections.GetAPISummary(context.Background(), args.CollectionID, 1, 20)
		if err != nil {
			return ExternalAPISummaryResult{}, err
		}
		var out ExternalAPISummaryResult
		out.Name = summary.Name
		for _, p := range summary.Paths {
			out.Paths = append(out.Paths, externalPathEntry(p))
		}
		return out, nil
	}
}

type ExternalAPIMethodArgs struct {
	CollectionID string `json:"collection_id" description:"The API collection ID"`
	Path         string `json:"path" description:"The API path, e.g. /users"`
	Method       string `json:"method" description:"HTTP method, e.g. get, post"`
}

func externalAPIMethod(deps *Deps) functiontool.Func[ExternalAPIMethodArgs, any] {
	return func(ctx agent.ToolContext, args ExternalAPIMethodArgs) (any, error) {
		detail, err := deps.APICollections.GetAPIMethod(context.Background(), args.CollectionID, args.Path, args.Method)
		if err != nil {
			return nil, err
		}
		return detail, nil
	}
}

type ExternalAPICallArgs struct {
	CollectionID string            `json:"collection_id" description:"The API collection ID"`
	Path         string            `json:"path" description:"The API path, e.g. /users"`
	Method       string            `json:"method" description:"HTTP method: get, post, put, delete"`
	Params       map[string]string `json:"params" description:"Query parameters, e.g. {\"page\":\"1\"}"`
	Body         any               `json:"body" description:"Request body for POST/PUT"`
}

func externalAPICall(deps *Deps) functiontool.Func[ExternalAPICallArgs, any] {
	return func(ctx agent.ToolContext, args ExternalAPICallArgs) (any, error) {
		result, err := deps.APICollections.CallAPI(context.Background(), args.CollectionID, args.Path, args.Method, args.Params, args.Body, nil)
		if err != nil {
			return nil, err
		}
		return result, nil
	}
}
