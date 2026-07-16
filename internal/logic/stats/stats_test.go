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
	s := formatFloat(3.14)
	if len(s) == 0 {
		t.Error("formatFloat should not be empty")
	}
}

func TestItoa(t *testing.T) {
	if s := itoa(0); s != "0" {
		t.Errorf("0: got %s", s)
	}
	if s := itoa(42); s != "42" {
		t.Errorf("42: got %s", s)
	}
}
