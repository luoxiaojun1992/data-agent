package skill

import (
	"fmt"

	statspkg "github.com/luoxiaojun1992/data-agent/internal/logic/stats"
	skilldomain "github.com/luoxiaojun1992/data-agent/internal/domain/skill"
)

// StatsEngine implements skill.Skill for statistical analysis.
type StatsEngine struct{}

func (s *StatsEngine) Name() string        { return "stats_engine" }
func (s *StatsEngine) Description() string { return "Performs statistical analysis: descriptive stats, linear regression, time series decomposition" }

func (s *StatsEngine) Parameters() []skilldomain.Parameter {
	return []skilldomain.Parameter{
		{Name: "method", Type: "string", Description: "Analysis method: descriptive, linear_regression, time_series", Required: true},
		{Name: "values", Type: "array", Description: "Array of numeric values for analysis", Required: true},
		{Name: "label", Type: "string", Description: "Optional label for descriptive stats", Required: false},
		{Name: "x_values", Type: "array", Description: "X values for linear regression", Required: false},
	}
}

func (s *StatsEngine) Permissions() []string {
	return []string{"skill:stats_engine"}
}

func (s *StatsEngine) RateLimit() *skilldomain.RateLimitConfig {
	return &skilldomain.RateLimitConfig{MaxRequests: 20, WindowSec: 60}
}

func (s *StatsEngine) Execute(ctx skilldomain.SkillContext, params map[string]any) (any, error) {
	method, ok := params["method"].(string)
	if !ok || method == "" {
		return nil, fmt.Errorf("stats_engine: missing required parameter 'method'")
	}

	values, err := toFloatSlice(params["values"])
	if err != nil {
		return nil, fmt.Errorf("stats_engine: invalid 'values': %w", err)
	}

	switch method {
	case "descriptive":
		label, _ := params["label"].(string)
		return statspkg.Descriptive(values, label), nil
	case "linear_regression":
		xValues, xErr := toFloatSlice(params["x_values"])
		if xErr != nil {
			return nil, fmt.Errorf("stats_engine: 'x_values' required for linear_regression: %w", xErr)
		}
		return statspkg.LinearRegression(xValues, values), nil
	case "time_series":
		return statspkg.TimeSeriesDecompose(values), nil
	default:
		return nil, fmt.Errorf("stats_engine: unknown method %q (valid: descriptive, linear_regression, time_series)", method)
	}
}

func toFloatSlice(raw interface{}) ([]float64, error) {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("expected array, got %T", raw)
	}
	result := make([]float64, len(arr))
	for i, v := range arr {
		switch n := v.(type) {
		case float64:
			result[i] = n
		case int:
			result[i] = float64(n)
		case int64:
			result[i] = float64(n)
		default:
			return nil, fmt.Errorf("element[%d]: expected number, got %T", i, v)
		}
	}
	return result, nil
}
