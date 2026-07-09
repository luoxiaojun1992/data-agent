package skill

import (
	"fmt"

	sqlpkg "github.com/luoxiaojun1992/data-agent/internal/logic/sql"
	skilldomain "github.com/luoxiaojun1992/data-agent/internal/domain/skill"
)

// SQLExecutor implements skill.Skill for SQL query execution.
type SQLExecutor struct{}

func (s *SQLExecutor) Name() string        { return "sql_executor" }
func (s *SQLExecutor) Description() string { return "Executes validated SQL SELECT queries with safety checks" }

func (s *SQLExecutor) Parameters() []skilldomain.Parameter {
	return []skilldomain.Parameter{
		{Name: "query", Type: "string", Description: "SQL SELECT statement to execute", Required: true},
		{Name: "params", Type: "array", Description: "Parameterized query bind values", Required: false},
	}
}

func (s *SQLExecutor) Permissions() []string {
	return []string{"skill:sql_executor"}
}

func (s *SQLExecutor) RateLimit() *skilldomain.RateLimitConfig {
	return &skilldomain.RateLimitConfig{MaxRequests: 30, WindowSec: 60}
}

func (s *SQLExecutor) Execute(ctx skilldomain.SkillContext, params map[string]any) (any, error) {
	query, ok := params["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("sql_executor: missing required parameter 'query'")
	}

	var bindParams []interface{}
	if raw, ok := params["params"].([]interface{}); ok {
		bindParams = raw
	}

	result := sqlpkg.Validate(query, bindParams)
	if !result.Allowed {
		return nil, fmt.Errorf("sql_executor: query rejected: %s", result.Reason)
	}

	return map[string]any{
		"status":  "validated",
		"query":   query,
		"message": "SQL query passed safety validation. Execute against your database.",
		"reason":  result.Reason,
	}, nil
}
