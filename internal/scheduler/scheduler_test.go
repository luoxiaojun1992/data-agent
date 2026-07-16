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

func TestParseCronExpr_Valid(t *testing.T) {
	dur, err := parseCronExpr("every_5m")
	if err != nil {
		t.Fatalf("parseCronExpr(every_5m): %v", err)
	}
	if dur != 5*time.Minute {
		t.Errorf("every_5m: got %v, want 5m", dur)
	}
}

func TestParseCronExpr_1h(t *testing.T) {
	dur, err := parseCronExpr("every_1h")
	if err != nil {
		t.Fatalf("parseCronExpr(every_1h): %v", err)
	}
	if dur != 1*time.Hour {
		t.Errorf("every_1h: got %v, want 1h", dur)
	}
}

func TestParseCronExpr_Invalid(t *testing.T) {
	_, err := parseCronExpr("invalid_format")
	if err == nil {
		t.Error("should error on invalid format")
	}
}

func TestParseCronExpr_Empty(t *testing.T) {
	_, err := parseCronExpr("")
	if err == nil {
		t.Error("should error on empty")
	}
}
