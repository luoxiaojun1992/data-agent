package stats

import (
	"math"
	"testing"
)

func TestDescriptive(t *testing.T) {
	t.Run("empty dataset", func(t *testing.T) {
		r := Descriptive(nil, "")
		if r.Method != "descriptive" {
			t.Errorf("Method: got %s", r.Method)
		}
		if len(r.Warnings) == 0 {
			t.Error("should have warning for empty dataset")
		}
	})

	t.Run("small dataset", func(t *testing.T) {
		r := Descriptive([]float64{1, 2, 3}, "")
		found := false
		for _, w := range r.Warnings {
			if len(w) > 0 {
				found = true
			}
		}
		if !found {
			t.Error("should warn about small sample")
		}
		result := r.Result.(map[string]interface{})
		if result["mean"].(float64) != 2.0 {
			t.Errorf("mean: got %v", result["mean"])
		}
	})

	t.Run("with label", func(t *testing.T) {
		r := Descriptive([]float64{10, 20, 30}, "Revenue")
		result := r.Result.(map[string]interface{})
		if result["label"] != "Revenue" {
			t.Errorf("label: got %v", result["label"])
		}
	})
}

func TestLinearRegression(t *testing.T) {
	t.Run("mismatched lengths", func(t *testing.T) {
		r := LinearRegression([]float64{1, 2}, []float64{3})
		if len(r.Warnings) == 0 {
			t.Error("should warn on mismatched input")
		}
	})

	t.Run("empty inputs", func(t *testing.T) {
		r := LinearRegression(nil, nil)
		if r.Method != "linear_regression" {
			t.Errorf("Method: got %s", r.Method)
		}
	})

	t.Run("perfect correlation", func(t *testing.T) {
		x := []float64{1, 2, 3, 4, 5}
		y := []float64{2, 4, 6, 8, 10}
		r := LinearRegression(x, y)
		result := r.Result.(map[string]interface{})
		if math.Abs(result["slope"].(float64)-2.0) > 0.001 {
			t.Errorf("slope: got %v", result["slope"])
		}
		if math.Abs(result["r_squared"].(float64)-1.0) > 0.001 {
			t.Errorf("r_squared: got %v, want ~1.0", result["r_squared"])
		}
	})
}

func TestTimeSeriesDecompose(t *testing.T) {
	t.Run("too few points", func(t *testing.T) {
		r := TimeSeriesDecompose([]float64{1, 2, 3})
		if len(r.Warnings) == 0 {
			t.Error("should warn on < 4 data points")
		}
	})

	t.Run("valid series", func(t *testing.T) {
		values := []float64{10, 12, 14, 16, 18, 20, 22, 24, 26, 28}
		r := TimeSeriesDecompose(values)
		result := r.Result.(map[string]interface{})
		trend := result["trend"].([]float64)
		if len(trend) == 0 {
			t.Error("trend should not be empty")
		}
	})

	t.Run("upward trend detected", func(t *testing.T) {
		values := []float64{10, 12, 15, 18, 22, 27, 33, 40, 48, 57}
		r := TimeSeriesDecompose(values)
		result := r.Result.(map[string]interface{})
		if result["direction"] != "上升" {
			t.Errorf("expected 上升, got %v", result["direction"])
		}
	})

	t.Run("downward trend detected", func(t *testing.T) {
		values := []float64{57, 48, 40, 33, 27, 22, 18, 15, 12, 10}
		r := TimeSeriesDecompose(values)
		result := r.Result.(map[string]interface{})
		if result["direction"] != "下降" {
			t.Errorf("expected 下降, got %v", result["direction"])
		}
	})

	t.Run("flat trend", func(t *testing.T) {
		values := []float64{10, 10, 10.1, 9.9, 10, 10.05, 10}
		r := TimeSeriesDecompose(values)
		result := r.Result.(map[string]interface{})
		if result["direction"] != "flat" {
			t.Errorf("expected flat, got %v", result["direction"])
		}
	})

	t.Run("odd length series", func(t *testing.T) {
		values := []float64{1, 2, 3, 4, 5, 6, 7}
		r := TimeSeriesDecompose(values)
		result := r.Result.(map[string]interface{})
		trend := result["trend"].([]float64)
		if len(trend) != len(values) {
			t.Errorf("trend length: got %d, want %d", len(trend), len(values))
		}
	})

	t.Run("even length series", func(t *testing.T) {
		values := []float64{1, 2, 3, 4, 5, 6, 7, 8}
		r := TimeSeriesDecompose(values)
		result := r.Result.(map[string]interface{})
		trend := result["trend"].([]float64)
		if len(trend) != len(values) {
			t.Errorf("trend length: got %d, want %d", len(trend), len(values))
		}
	})

	t.Run("minimum 4 points window 2", func(t *testing.T) {
		values := []float64{1, 2, 3, 4}
		r := TimeSeriesDecompose(values)
		result := r.Result.(map[string]interface{})
		params := r.Parameters
		if params["window"].(int) != 2 {
			t.Errorf("expected window=2 for 4 points, got %d", params["window"])
		}
		if result["trend"] == nil {
			t.Error("trend should not be nil")
		}
	})

	t.Run("window calculation small dataset", func(t *testing.T) {
		// 6 points → len/2 = 3, which is < 7, so window = 3
		values := make([]float64, 6)
		for i := range values {
			values[i] = float64(i + 1)
		}
		r := TimeSeriesDecompose(values)
		params := r.Parameters
		if params["window"].(int) != 3 {
			t.Errorf("expected window=3 for 6 points, got %d", params["window"])
		}
	})

	t.Run("window calculation large dataset", func(t *testing.T) {
		// 20 points → len/2 = 10, which is > 7, so window = 7
		values := make([]float64, 20)
		for i := range values {
			values[i] = float64(i + 1)
		}
		r := TimeSeriesDecompose(values)
		params := r.Parameters
		if params["window"].(int) != 7 {
			t.Errorf("expected window=7 for 20 points, got %d", params["window"])
		}
	})

	t.Run("residuals match length", func(t *testing.T) {
		values := []float64{10, 12, 14, 16, 18}
		r := TimeSeriesDecompose(values)
		result := r.Result.(map[string]interface{})
		residuals := result["residuals"].([]float64)
		if len(residuals) != len(values) {
			t.Errorf("residuals length: got %d, want %d", len(residuals), len(values))
		}
	})

	t.Run("original preserved", func(t *testing.T) {
		values := []float64{10, 12, 14, 16, 18}
		r := TimeSeriesDecompose(values)
		result := r.Result.(map[string]interface{})
		original := result["original"].([]float64)
		if len(original) != len(values) {
			t.Errorf("original length: got %d, want %d", len(original), len(values))
		}
		for i, v := range original {
			if v != values[i] {
				t.Errorf("original[%d]: got %v, want %v", i, v, values[i])
			}
		}
	})

	t.Run("summary contains direction", func(t *testing.T) {
		values := []float64{10, 12, 14, 16, 18, 20, 22}
		r := TimeSeriesDecompose(values)
		if r.Summary == "" {
			t.Error("summary should not be empty")
		}
	})
}

func TestSum(t *testing.T) {
	if s := sum([]float64{1, 2, 3}); s != 6 {
		t.Errorf("sum: got %v, want 6", s)
	}
	if s := sum(nil); s != 0 {
		t.Errorf("sum nil: got %v, want 0", s)
	}
}

func TestSafeDiv(t *testing.T) {
	if d := safeDiv(10, 2); d != 5 {
		t.Errorf("10/2: got %v", d)
	}
	if d := safeDiv(10, 0); d != 0 {
		t.Errorf("10/0: got %v, want 0", d)
	}
}

func TestCalcVariance(t *testing.T) {
	v := calcVariance([]float64{1, 2, 3}, 2)
	if v <= 0 {
		t.Errorf("variance should be positive: got %v", v)
	}
	if v1 := calcVariance([]float64{1}, 1); v1 != 0 {
		t.Errorf("variance of single value: got %v", v1)
	}
}

func TestPercentile(t *testing.T) {
	sorted := []float64{1, 2, 3, 4, 5}
	if p := percentile(sorted, 0.5); p != 3 {
		t.Errorf("median: got %v", p)
	}
	if p := percentile(sorted, 0); p != 1 {
		t.Errorf("p0: got %v", p)
	}
	if p := percentile(nil, 0.5); p != 0 {
		t.Errorf("empty: got %v", p)
	}
}

func TestFormatFloat(t *testing.T) {
	tests := []struct {
		name string
		f    float64
		want string
	}{
		{"zero", 0.0, "0.0000"},
		{"one", 1.0, "1.0000"},
		{"pi", 3.14, "3.1400"},
		{"1.5", 1.5, "1.5000"},
		{"-1.5", -1.5, "-1.5000"},
		{"0.001", 0.001, "0.0010"},
		{"0.0001", 0.0001, "0.0001"},
		{"large int", 12345.0, "12345.0000"},
		{"negative", -3.14, "-3.1400"},
		{"decimal rounding", 1.23456, "1.2345"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatFloat(tt.f)
			if got != tt.want {
				t.Errorf("formatFloat(%v) = %q, want %q", tt.f, got, tt.want)
			}
		})
	}
}

func TestTimeSeriesDecompose_NegativeTrend(t *testing.T) {
	// Test with negative values — all getting less negative (upward trend)
	values := []float64{-100, -95, -90, -85, -80, -75, -70, -65, -60, -55}
	r := TimeSeriesDecompose(values)
	result := r.Result.(map[string]interface{})
	if result["direction"] != "上升" {
		t.Errorf("expected 上升 for negative values trending up, got %v", result["direction"])
	}
}

func TestTimeSeriesDecompose_BoundaryThreshold(t *testing.T) {
	// Values where last trend is exactly 5% above first → should be "flat" (not > 1.05)
	values := []float64{100, 100, 100, 100, 100, 100, 100, 100, 100, 100}
	r := TimeSeriesDecompose(values)
	result := r.Result.(map[string]interface{})
	if result["direction"] != "flat" {
		t.Errorf("expected flat for constant values, got %v", result["direction"])
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want string
	}{
		{"zero", 0, "0"},
		{"single digit 1", 1, "1"},
		{"single digit 9", 9, "9"},
		{"two digits", 42, "42"},
		{"three digits", 100, "100"},
		{"four digits", 1234, "1234"},
		{"large", 99999, "99999"},
		{"ten", 10, "10"},
		{"hundred", 1000, "1000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := itoa(tt.n)
			if got != tt.want {
				t.Errorf("itoa(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}
