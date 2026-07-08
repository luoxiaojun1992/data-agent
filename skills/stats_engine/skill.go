package stats_engine

import (
	"github.com/luoxiaojun1992/data-agent/internal/domain/skill"
	"github.com/luoxiaojun1992/data-agent/internal/logic/stats"
)

type Engine struct{}

func (e *Engine) Name() string        { return "stats_engine" }
func (e *Engine) Description() string { return "Performs statistical analysis: descriptive, regression, time series" }

func (e *Engine) Parameters() []skill.Parameter {
	return []skill.Parameter{
		{Name: "method", Type: "string", Description: "Analysis method: descriptive, regression, timeseries", Required: true},
		{Name: "data", Type: "array", Description: "Array of numeric values for analysis", Required: true},
		{Name: "labels", Type: "array", Description: "Optional labels for each data point", Required: false},
	}
}

func (e *Engine) Permissions() []string { return []string{"kb:manage_own"} }
func (e *Engine) RateLimit() *skill.RateLimitConfig { return &skill.RateLimitConfig{MaxRequests: 5, WindowSec: 60} }

func (e *Engine) Execute(ctx skill.SkillContext, params map[string]any) (any, error) {
	method, _ := params["method"].(string)
	dataRaw, _ := params["data"].([]interface{})

	values := make([]float64, len(dataRaw))
	for i, v := range dataRaw {
		if f, ok := v.(float64); ok {
			values[i] = f
		}
	}

	_ = ctx

	switch method {
	case "regression", "linear_regression":
		return stats.LinearRegression(values, values), nil
	case "timeseries", "time_series":
		return stats.TimeSeriesDecompose(values), nil
	default:
		return stats.Descriptive(values, ""), nil
	}
}
