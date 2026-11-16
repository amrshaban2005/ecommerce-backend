// Package circuit provides a simple circuit breaker for downstream services.
package circuit

import (
	"log/slog"
	"sync"
	"time"
)

// State represents the circuit breaker state.
type State int

const (
	StateClosed   State = iota // Normal operation
	StateOpen                  // Rejecting requests
	StateHalfOpen              // Probe: allow one request through
)

// Breaker is a per-service circuit breaker.
// Threshold: 5 consecutive failures → OPEN.
// Recovery: 10s → HALF-OPEN → probe → CLOSED on success.
type Breaker struct {
	mu           sync.Mutex
	state        State
	failures     int
	threshold    int
	openUntil    time.Time
	halfOpenWait time.Duration
	serviceName  string
}

// Option configures a Breaker.
type Option func(*Breaker)

// WithHalfOpenWait overrides the default recovery window.
func WithHalfOpenWait(wait time.Duration) Option {
	return func(b *Breaker) {
		if wait > 0 {
			b.halfOpenWait = wait
		}
	}
}

// New creates a Breaker with the standard config (5 failures, 10s recovery).
func New(serviceName string, opts ...Option) *Breaker {
	b := &Breaker{
		state:        StateClosed,
		threshold:    5,
		halfOpenWait: 10 * time.Second,
		serviceName:  serviceName,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Allow returns true if the request should proceed.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Now().After(b.openUntil) {
			// Transition to half-open: allow one probe
			b.state = StateHalfOpen
			slog.Info("circuit breaker half-open",
				"service", b.serviceName,
			)
			return true
		}
		return false
	case StateHalfOpen:
		// Only one probe at a time
		return false
	}
	return false
}

// Success records a successful call.
func (b *Breaker) Success() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateClosed:
		b.failures = 0
	case StateHalfOpen:
		b.failures = 0
		slog.Info("circuit breaker closed",
			"service", b.serviceName,
		)
		b.state = StateClosed
	case StateOpen:
		// Ignore successes from requests that were already in flight before the circuit opened.
		return
	}
}

// Failure records a failed call.
func (b *Breaker) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.failures++
	if b.state == StateHalfOpen || b.failures >= b.threshold {
		b.state = StateOpen
		b.openUntil = time.Now().Add(b.halfOpenWait)
		slog.Warn("circuit breaker opened",
			"service", b.serviceName,
			"failures", b.failures,
			"retry_after_sec", b.halfOpenWait.Seconds(),
		)
		b.failures = 0
	}
}

// IsOpen returns true when circuit is open (rejecting requests).
func (b *Breaker) IsOpen() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state == StateOpen && time.Now().Before(b.openUntil)
}
