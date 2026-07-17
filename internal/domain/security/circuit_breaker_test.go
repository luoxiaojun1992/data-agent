package security

import (
	"fmt"
	"testing"
	"time"
)

func TestCircuitState_String(t *testing.T) {
	tests := []struct {
		state CircuitState
		want  string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}
	for _, tt := range tests {
		got := tt.state.String()
		if got != tt.want {
			t.Errorf("CircuitState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()
	if cfg.MaxFailures != 5 {
		t.Errorf("MaxFailures: got %d, want 5", cfg.MaxFailures)
	}
	if cfg.CooldownSec != 30 {
		t.Errorf("CooldownSec: got %d, want 30", cfg.CooldownSec)
	}
	if cfg.TimeoutSec != 10 {
		t.Errorf("TimeoutSec: got %d, want 10", cfg.TimeoutSec)
	}
}

func TestCircuitBreaker_Call(t *testing.T) {
	// Use a config with very short cooldown for fast testing
	cfg := CircuitBreakerConfig{MaxFailures: 2, CooldownSec: 1, TimeoutSec: 0}

	t.Run("closed state success resets failures", func(t *testing.T) {
		cb := NewCircuitBreaker(cfg)
		cb.failures = 3 // simulate previous failures
		err := cb.Call(func() error { return nil })
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if cb.Failures() != 0 {
			t.Errorf("Failures should reset to 0, got %d", cb.Failures())
		}
		if cb.State() != StateClosed {
			t.Errorf("State should be Closed, got %s", cb.State())
		}
	})

	t.Run("too many failures opens circuit", func(t *testing.T) {
		cb := NewCircuitBreaker(cfg)
		testErr := fmt.Errorf("test error")
		// MaxFailures is 2, so 2 consecutive failures should open
		for i := 0; i < 2; i++ {
			err := cb.Call(func() error { return testErr })
			if err != testErr {
				t.Fatalf("iteration %d: expected testErr, got %v", i, err)
			}
		}
		if cb.State() != StateOpen {
			t.Errorf("State should be Open after %d failures, got %s", cfg.MaxFailures, cb.State())
		}
	})

	t.Run("open state returns error immediately", func(t *testing.T) {
		cb := NewCircuitBreaker(cfg)
		cb.state = StateOpen
		cb.lastState = time.Now()

		called := false
		err := cb.Call(func() error { called = true; return nil })
		if called {
			t.Error("function should NOT be called when circuit is open")
		}
		if err == nil {
			t.Error("should return error when circuit is open")
		}
	})

	t.Run("open state transitions to half-open after cooldown", func(t *testing.T) {
		cfg := CircuitBreakerConfig{MaxFailures: 2, CooldownSec: 0, TimeoutSec: 0}
		cb := NewCircuitBreaker(cfg)
		cb.state = StateOpen
		cb.lastState = time.Now().Add(-time.Second) // simulate open 1 second ago

		called := false
		err := cb.Call(func() error { called = true; return nil })
		if !called {
			t.Error("function should be called when cooldown expired")
		}
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if cb.State() != StateClosed {
			t.Errorf("State should be Closed after success in half-open, got %s", cb.State())
		}
	})

	t.Run("half-open failure reopens circuit", func(t *testing.T) {
		cfg := CircuitBreakerConfig{MaxFailures: 2, CooldownSec: 0, TimeoutSec: 0}
		cb := NewCircuitBreaker(cfg)
		cb.state = StateOpen
		cb.lastState = time.Now().Add(-time.Second)

		testErr := fmt.Errorf("test error")
		err := cb.Call(func() error { return testErr })
		if err != testErr {
			t.Errorf("expected testErr, got %v", err)
		}
		if cb.State() != StateOpen {
			t.Errorf("State should be Open after failure in half-open, got %s", cb.State())
		}
	})
}

func TestCircuitBreaker_State(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())
	if cb.State() != StateClosed {
		t.Errorf("new breaker should be Closed, got %s", cb.State())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())
	cb.failures = 5
	cb.state = StateOpen

	cb.Reset()
	if cb.State() != StateClosed {
		t.Errorf("after Reset: got %s, want Closed", cb.State())
	}
	if cb.Failures() != 0 {
		t.Errorf("after Reset: got %d failures, want 0", cb.Failures())
	}
}

func TestCircuitBreakerRegistry(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()
	reg := NewCircuitBreakerRegistry(cfg)

	t.Run("GetOrCreate creates on first call", func(t *testing.T) {
		cb1 := reg.GetOrCreate("skill-a")
		if cb1 == nil {
			t.Fatal("GetOrCreate returned nil")
		}
	})

	t.Run("GetOrCreate returns same instance", func(t *testing.T) {
		cb1 := reg.GetOrCreate("skill-b")
		cb2 := reg.GetOrCreate("skill-b")
		if cb1 != cb2 {
			t.Error("GetOrCreate should return same instance")
		}
	})

	t.Run("different skills get different breakers", func(t *testing.T) {
		cb1 := reg.GetOrCreate("skill-c")
		cb2 := reg.GetOrCreate("skill-d")
		if cb1 == cb2 {
			t.Error("different skills should get different breakers")
		}
	})

	t.Run("List returns states", func(t *testing.T) {
		// Clear and add known skills
		reg2 := NewCircuitBreakerRegistry(cfg)
		reg2.GetOrCreate("skill-x")
		reg2.GetOrCreate("skill-y")

		states := reg2.List()
		if len(states) != 2 {
			t.Errorf("List: got %d entries, want 2", len(states))
		}
	})
}
