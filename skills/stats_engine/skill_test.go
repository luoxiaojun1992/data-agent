package skill

import (
	"testing"

	skilldomain "github.com/luoxiaojun1992/data-agent/internal/domain/skill"
)

func TestStatsEngine_Name(t *testing.T) {
	s := &StatsEngine{}
	if got := s.Name(); got != "stats_engine" {
		t.Errorf("Name() = %q, want %q", got, "stats_engine")
	}
}

func TestStatsEngine_Description(t *testing.T) {
	s := &StatsEngine{}
	desc := s.Description()
	if desc == "" {
		t.Error("Description() should not be empty")
	}
}

func TestStatsEngine_Parameters(t *testing.T) {
	s := &StatsEngine{}
	params := s.Parameters()
	if len(params) < 2 {
		t.Fatal("Parameters() should return at least 2 parameters")
	}
	if params[0].Name != "method" {
		t.Errorf("first param name = %q, want %q", params[0].Name, "method")
	}
	if !params[0].Required {
		t.Error("'method' parameter should be required")
	}
	if params[1].Name != "values" {
		t.Errorf("second param name = %q, want %q", params[1].Name, "values")
	}
}

func TestStatsEngine_Permissions(t *testing.T) {
	s := &StatsEngine{}
	perms := s.Permissions()
	if len(perms) == 0 {
		t.Error("Permissions() should not be empty")
	}
	found := false
	for _, p := range perms {
		if p == "skill:stats_engine" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Permissions() should contain 'skill:stats_engine'")
	}
}

func TestStatsEngine_RateLimit(t *testing.T) {
	s := &StatsEngine{}
	rl := s.RateLimit()
	if rl == nil {
		t.Fatal("RateLimit() should not be nil")
	}
	if rl.MaxRequests != 20 {
		t.Errorf("MaxRequests = %d, want 20", rl.MaxRequests)
	}
	if rl.WindowSec != 60 {
		t.Errorf("WindowSec = %d, want 60", rl.WindowSec)
	}
}

func TestStatsEngine_Execute_MissingMethod(t *testing.T) {
	s := &StatsEngine{}
	ctx := skilldomain.SkillContext{}
	result, err := s.Execute(ctx, map[string]any{})
	if err == nil {
		t.Error("Execute() should return error for missing 'method'")
	}
	if result != nil {
		t.Error("Execute() should return nil result on error")
	}
}

func TestStatsEngine_Execute_EmptyMethod(t *testing.T) {
	s := &StatsEngine{}
	ctx := skilldomain.SkillContext{}
	result, err := s.Execute(ctx, map[string]any{"method": ""})
	if err == nil {
		t.Error("Execute() should return error for empty 'method'")
	}
	if result != nil {
		t.Error("Execute() should return nil result on error")
	}
}

func TestStatsEngine_Execute_InvalidValuesType(t *testing.T) {
	s := &StatsEngine{}
	ctx := skilldomain.SkillContext{}
	result, err := s.Execute(ctx, map[string]any{
		"method": "descriptive",
		"values": "not an array",
	})
	if err == nil {
		t.Error("Execute() should return error for invalid 'values' type")
	}
	if result != nil {
		t.Error("Execute() should return nil result on error")
	}
}

func TestStatsEngine_Execute_UnknownMethod(t *testing.T) {
	s := &StatsEngine{}
	ctx := skilldomain.SkillContext{}
	result, err := s.Execute(ctx, map[string]any{
		"method": "unknown_method",
		"values": []interface{}{1.0, 2.0, 3.0},
	})
	if err == nil {
		t.Error("Execute() should return error for unknown method")
	}
	if result != nil {
		t.Error("Execute() should return nil result on error")
	}
}

func TestStatsEngine_Execute_Descriptive(t *testing.T) {
	s := &StatsEngine{}
	ctx := skilldomain.SkillContext{UserID: "user1", Role: "user"}
	result, err := s.Execute(ctx, map[string]any{
		"method": "descriptive",
		"values": []interface{}{1.0, 2.0, 3.0, 4.0, 5.0},
		"label":  "test_data",
	})
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Execute() should return result for valid input")
	}
}

func TestStatsEngine_Execute_LinearRegression(t *testing.T) {
	s := &StatsEngine{}
	ctx := skilldomain.SkillContext{UserID: "user1", Role: "user"}
	result, err := s.Execute(ctx, map[string]any{
		"method":  "linear_regression",
		"values":  []interface{}{2.0, 4.0, 6.0},
		"x_values": []interface{}{1.0, 2.0, 3.0},
	})
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Execute() should return result for valid linear regression")
	}
}

func TestStatsEngine_Execute_LinearRegression_MissingXValues(t *testing.T) {
	s := &StatsEngine{}
	ctx := skilldomain.SkillContext{}
	result, err := s.Execute(ctx, map[string]any{
		"method": "linear_regression",
		"values": []interface{}{2.0, 4.0, 6.0},
	})
	if err == nil {
		t.Error("Execute() should return error when x_values missing for linear_regression")
	}
	if result != nil {
		t.Error("Execute() should return nil result on error")
	}
}

func TestStatsEngine_Execute_TimeSeries(t *testing.T) {
	s := &StatsEngine{}
	ctx := skilldomain.SkillContext{UserID: "user1", Role: "user"}
	result, err := s.Execute(ctx, map[string]any{
		"method": "time_series",
		"values": []interface{}{10.0, 12.0, 15.0, 14.0, 16.0, 18.0},
	})
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Execute() should return result for valid time_series")
	}
}

func TestToFloatSlice(t *testing.T) {
	t.Run("float64 values", func(t *testing.T) {
		result, err := toFloatSlice([]interface{}{1.0, 2.0, 3.0})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 3 {
			t.Fatalf("len = %d, want 3", len(result))
		}
		if result[0] != 1.0 || result[1] != 2.0 || result[2] != 3.0 {
			t.Errorf("values mismatch: got %v", result)
		}
	})

	t.Run("int values", func(t *testing.T) {
		result, err := toFloatSlice([]interface{}{1, 2, 3})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 3 {
			t.Fatalf("len = %d, want 3", len(result))
		}
	})

	t.Run("int64 values", func(t *testing.T) {
		result, err := toFloatSlice([]interface{}{int64(10), int64(20)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("len = %d, want 2", len(result))
		}
	})

	t.Run("empty array", func(t *testing.T) {
		result, err := toFloatSlice([]interface{}{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Fatalf("len = %d, want 0", len(result))
		}
	})

	t.Run("non-array input", func(t *testing.T) {
		_, err := toFloatSlice("not an array")
		if err == nil {
			t.Error("should return error for non-array input")
		}
	})

	t.Run("non-numeric element", func(t *testing.T) {
		_, err := toFloatSlice([]interface{}{1.0, "string", 3.0})
		if err == nil {
			t.Error("should return error for non-numeric element")
		}
	})

	t.Run("mixed int and float", func(t *testing.T) {
		result, err := toFloatSlice([]interface{}{1, 2.0, int64(3)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 3 {
			t.Fatalf("len = %d, want 3", len(result))
		}
	})
}
