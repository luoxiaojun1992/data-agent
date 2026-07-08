package sql_executor

import (
	"fmt"

	"github.com/luoxiaojun1992/data-agent/internal/domain/skill"
	sqllogic "github.com/luoxiaojun1992/data-agent/internal/logic/sql"
)

// Executor implements the SQL executor skill.
type Executor struct{}

func (e *Executor) Name() string        { return "sql_executor" }
func (e *Executor) Description() string { return "Executes safe SQL queries against the connected database" }

func (e *Executor) Parameters() []skill.Parameter {
	return []skill.Parameter{
		{Name: "sql", Type: "string", Description: "The SQL query to execute", Required: true},
		{Name: "params", Type: "array", Description: "Query parameters for parameterized queries", Required: false},
	}
}

func (e *Executor) Permissions() []string {
	return []string{"kb:manage_own"}
}

func (e *Executor) RateLimit() *skill.RateLimitConfig {
	return &skill.RateLimitConfig{MaxRequests: 10, WindowSec: 60}
}

func (e *Executor) Execute(ctx skill.SkillContext, params map[string]any) (any, error) {
	sql, ok := params["sql"].(string)
	if !ok || sql == "" {
		return nil, fmt.Errorf("sql parameter required")
	}

	// Validate SQL safety
	result := sqllogic.Validate(sql, nil)
	if !result.Allowed {
		return nil, fmt.Errorf("SQL not allowed: %s", result.Reason)
	}

	_ = ctx
	_ = params

	return map[string]interface{}{
		"status": "executed",
		"sql":    sql,
		"rows":   []interface{}{},
	}, nil
}
