package stats

import (
	"math"
	"sort"
)

// AnalysisResult is the unified output structure for all stats functions.
type AnalysisResult struct {
	Method     string        `json:"method"`
	Parameters map[string]interface{} `json:"parameters"`
	Result     interface{}   `json:"result"`
	Summary    string        `json:"summary"`
	Warnings   []string      `json:"warnings"`
}

// Descriptive computes descriptive statistics for a slice of values.
func Descriptive(values []float64, label string) *AnalysisResult {
	if len(values) == 0 {
		return &AnalysisResult{Method: "descriptive", Warnings: []string{"empty dataset"}}
	}

	warnings := []string{}
	if len(values) < 30 {
		warnings = append(warnings, "样本量 < 30，结果可能不稳定")
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	n := float64(len(values))
	mean := safeDiv(sum(values), n)
	variance := calcVariance(values, mean)
	stdDev := math.Sqrt(variance)

	median := percentile(sorted, 0.5)
	q1 := percentile(sorted, 0.25)
	q3 := percentile(sorted, 0.75)

	result := map[string]interface{}{
		"count":    len(values),
		"mean":     mean,
		"median":   median,
		"std_dev":  stdDev,
		"variance": variance,
		"min":      sorted[0],
		"max":      sorted[len(sorted)-1],
		"q1":       q1,
		"q3":       q3,
	}

	if label != "" {
		result["label"] = label
	}

	return &AnalysisResult{
		Method:     "descriptive",
		Parameters: map[string]interface{}{"count": len(values), "label": label},
		Result:     result,
		Summary:    formatDescriptiveSummary(label, mean, median, stdDev),
		Warnings:   warnings,
	}
}

// LinearRegression performs simple linear regression on (x, y) data pairs.
func LinearRegression(x, y []float64) *AnalysisResult {
	if len(x) != len(y) || len(x) == 0 {
		return &AnalysisResult{Method: "linear_regression", Warnings: []string{"invalid or mismatched input data"}}
	}

	warnings := []string{}
	if len(x) < 30 {
		warnings = append(warnings, "样本量 < 30，回归结果可能不稳定")
	}

	n := float64(len(x))
	sumX := sum(x)
	sumY := sum(y)
	sumXY := 0.0
	sumX2 := 0.0
	for i := range x {
		sumXY += x[i] * y[i]
		sumX2 += x[i] * x[i]
	}

	slope := safeDiv(n*sumXY-sumX*sumY, n*sumX2-sumX*sumX)
	intercept := safeDiv(sumY-slope*sumX, n)

	// Calculate R-squared
	meanY := safeDiv(sumY, n)
	ssRes := 0.0
	ssTot := 0.0
	for i := range y {
		predicted := slope*x[i] + intercept
		ssRes += (y[i] - predicted) * (y[i] - predicted)
		ssTot += (y[i] - meanY) * (y[i] - meanY)
	}
	rSquared := 1 - safeDiv(ssRes, ssTot)

	return &AnalysisResult{
		Method: "linear_regression",
		Parameters: map[string]interface{}{"data_points": len(x)},
		Result: map[string]interface{}{
			"slope":     slope,
			"intercept": intercept,
			"r_squared": rSquared,
			"equation":  formatEquation(slope, intercept),
		},
		Summary:  formatRegressionSummary(slope, intercept, rSquared),
		Warnings: warnings,
	}
}

// TimeSeriesDecompose decomposes a time series into trend + residuals.
func TimeSeriesDecompose(values []float64) *AnalysisResult {
	if len(values) < 4 {
		return &AnalysisResult{Method: "time_series", Warnings: []string{"需要至少 4 个数据点"}}
	}

	// Simple moving average as trend (window = min(7, len/2))
	window := 7
	if len(values)/2 < window {
		window = len(values) / 2
	}
	if window < 2 {
		window = 2
	}

	trend := movingAverage(values, window)
	residuals := make([]float64, len(values))
	for i := range values {
		residuals[i] = values[i] - trend[i]
	}

	// Detect if there's a clear upward/downward trend
	trendDirection := "flat"
	if len(trend) >= 2 {
		if trend[len(trend)-1] > trend[0]*1.05 {
			trendDirection = "上升"
		} else if trend[len(trend)-1] < trend[0]*0.95 {
			trendDirection = "下降"
		}
	}

	return &AnalysisResult{
		Method:     "time_series",
		Parameters: map[string]interface{}{"data_points": len(values), "window": window},
		Result: map[string]interface{}{
			"trend":      trend,
			"residuals":  residuals,
			"direction":  trendDirection,
			"original":   values,
		},
		Summary: "时间序列分解完成，趋势方向：" + trendDirection,
	}
}

// ── Helpers ──

func sum(values []float64) float64 {
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

func calcVariance(values []float64, mean float64) float64 {
	if len(values) <= 1 {
		return 0
	}
	sumSq := 0.0
	for _, v := range values {
		diff := v - mean
		sumSq += diff * diff
	}
	return sumSq / float64(len(values)-1)
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p * float64(len(sorted)-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	if lo == hi || hi >= len(sorted) {
		return sorted[lo]
	}
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

func movingAverage(values []float64, window int) []float64 {
	result := make([]float64, len(values))
	for i := range values {
		start := i - window/2
		end := i + window/2 + 1
		if start < 0 {
			start = 0
		}
		if end > len(values) {
			end = len(values)
		}
		sum := 0.0
		for j := start; j < end; j++ {
			sum += values[j]
		}
		result[i] = sum / float64(end-start)
	}
	return result
}

func formatDescriptiveSummary(label string, mean, median, stdDev float64) string {
	if label != "" {
		return label + " 描述统计：均值=" + formatFloat(mean) + "，中位数=" + formatFloat(median) + "，标准差=" + formatFloat(stdDev)
	}
	return "描述统计：均值=" + formatFloat(mean) + "，中位数=" + formatFloat(median)
}

func formatRegressionSummary(slope, intercept, rSquared float64) string {
	return "回归方程: y = " + formatFloat(slope) + "x + " + formatFloat(intercept) + " (R²=" + formatFloat(rSquared) + ")"
}

func formatEquation(slope, intercept float64) string {
	return "y = " + formatFloat(slope) + "x + " + formatFloat(intercept)
}

func formatFloat(f float64) string {
	result := ""
	n := int(math.Abs(f) * 10000)
	intPart := n / 10000
	decPart := n % 10000

	if f < 0 {
		result = "-"
	}
	result += itoa(intPart) + "."
	// Pad decimal part
	for d := decPart; d < 1000 && decPart > 0; d *= 10 {
		result += "0"
	}
	if decPart > 0 {
		result += itoa(decPart)
	} else {
		result += "0000"
	}
	return result
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
