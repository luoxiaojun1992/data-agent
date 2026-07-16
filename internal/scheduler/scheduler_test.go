package scheduler

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	s := New(nil)
	if s == nil {
		t.Error("New should not return nil")
	}
}

func TestScheduleDefaults(t *testing.T) {
	s := Schedule{
		ID:      "sched-1",
		Name:    "test",
		Enabled: true,
	}
	if s.ID != "sched-1" {
		t.Errorf("ID: got %s", s.ID)
	}
	if !s.Enabled {
		t.Error("Enabled should be true")
	}
}

func TestParseCronExpr(t *testing.T) {
	tests := []struct {
		expr        string
		wantMinZero bool // interval should be > 0 for valid expressions
		wantErr     bool
	}{
		{"every_5m", false, false},
		{"every_1h", false, false},
		{"invalid", false, true},
	}

	for _, tt := range tests {
		got, err := parseCronExpr(tt.expr)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseCronExpr(%q) should error", tt.expr)
			}
		} else {
			if err != nil {
				t.Errorf("parseCronExpr(%q) unexpected error: %v", tt.expr, err)
			}
			if tt.wantMinZero && got != 0 {
				t.Errorf("parseCronExpr(%q) = %v, want 0", tt.expr, got)
			}
			if !tt.wantMinZero && got <= 0 {
				t.Errorf("parseCronExpr(%q) = %v, want > 0", tt.expr, got)
			}
		}
	}
}

func TestParseCronExpr_ValidDurations(t *testing.T) {
	dur, err := parseCronExpr("every_5m")
	if err != nil {
		t.Fatal(err)
	}
	if dur != 5*time.Minute {
		t.Errorf("every_5m: got %v, want 5m", dur)
	}

	dur, err = parseCronExpr("every_1h")
	if err != nil {
		t.Fatal(err)
	}
	if dur != 1*time.Hour {
		t.Errorf("every_1h: got %v, want 1h", dur)
	}
}
