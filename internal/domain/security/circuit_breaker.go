package security

import (
	"fmt"
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	StateClosed   CircuitState = iota // Normal operation
	StateOpen                         // Refusing requests
	StateHalfOpen                     // Testing recovery
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig defines configuration for a circuit breaker.
type CircuitBreakerConfig struct {
	MaxFailures int // Consecutive failures to open the circuit
	CooldownSec int // Seconds before transitioning from open to half-open
	TimeoutSec  int // Timeout for half-open probe requests
}

// DefaultCircuitBreakerConfig returns recommended defaults.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		MaxFailures: 5,
		CooldownSec: 30,
		TimeoutSec:  10,
	}
}

// CircuitBreaker implements the circuit breaker pattern per skill.
type CircuitBreaker struct {
	mu        sync.Mutex
	config    CircuitBreakerConfig
	state     CircuitState
	failures  int
	lastFail  time.Time
	lastState time.Time
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		config:    cfg,
		state:     StateClosed,
		lastState: time.Now(),
	}
}

// Call executes the given function with circuit breaker protection.
func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mu.Lock()
	state := cb.state

	switch state {
	case StateOpen:
		if time.Since(cb.lastState) > time.Duration(cb.config.CooldownSec)*time.Second {
			cb.state = StateHalfOpen
			cb.lastState = time.Now()
		} else {
			cb.mu.Unlock()
			return fmt.Errorf("circuit breaker is open (cooldown: %ds remaining)",
				cb.config.CooldownSec-int(time.Since(cb.lastState).Seconds()))
		}
	case StateHalfOpen:
		// Allow only one probe request
	default:
		// StateClosed — proceed normally
	}
	cb.mu.Unlock()

	// Execute
	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failures++
		cb.lastFail = time.Now()

		switch cb.state {
		case StateHalfOpen:
			cb.state = StateOpen
			cb.lastState = time.Now()
		case StateClosed:
			if cb.failures >= cb.config.MaxFailures {
				cb.state = StateOpen
				cb.lastState = time.Now()
			}
		}
		return err
	}

	// Success — reset
	cb.failures = 0
	if cb.state == StateHalfOpen {
		cb.state = StateClosed
		cb.lastState = time.Now()
	}
	return nil
}

// State returns the current circuit breaker state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// Failures returns the current failure count.
func (cb *CircuitBreaker) Failures() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.failures
}

// Reset manually resets the circuit breaker to closed.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateClosed
	cb.failures = 0
}

// CircuitBreakerRegistry manages circuit breakers per skill.
type CircuitBreakerRegistry struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
	config   CircuitBreakerConfig
}

// NewCircuitBreakerRegistry creates a new registry.
func NewCircuitBreakerRegistry(cfg CircuitBreakerConfig) *CircuitBreakerRegistry {
	return &CircuitBreakerRegistry{
		breakers: make(map[string]*CircuitBreaker),
		config:   cfg,
	}
}

// GetOrCreate returns the circuit breaker for a skill, creating it if needed.
func (r *CircuitBreakerRegistry) GetOrCreate(skillName string) *CircuitBreaker {
	r.mu.Lock()
	defer r.mu.Unlock()

	if cb, exists := r.breakers[skillName]; exists {
		return cb
	}
	cb := NewCircuitBreaker(r.config)
	r.breakers[skillName] = cb
	return cb
}

// List returns the state of all circuit breakers.
func (r *CircuitBreakerRegistry) List() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]string)
	for name, cb := range r.breakers {
		result[name] = cb.State().String()
	}
	return result
}
