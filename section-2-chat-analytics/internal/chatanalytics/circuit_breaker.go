package chatanalytics

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	StateClosed   = 0
	StateOpen     = 1
	StateHalfOpen = 2
)

type CircuitBreaker struct {
	maxFailures  int
	resetTimeout time.Duration
	failures     atomic.Int64
	state        atomic.Int32
	lastFailTime time.Time
	mu           sync.RWMutex
}

func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	cb := &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
	}
	cb.state.Store(StateClosed)
	return cb
}

func (cb *CircuitBreaker) Call(fn func() error) error {
	state := cb.state.Load()

	if state == StateOpen {
		cb.mu.RLock()
		elapsed := time.Since(cb.lastFailTime)
		cb.mu.RUnlock()

		if elapsed > cb.resetTimeout {
			cb.state.Store(StateHalfOpen)
		} else {
			return fmt.Errorf("circuit breaker open")
		}
	}

	err := fn()

	if err != nil {
		cb.failures.Add(1)
		cb.mu.Lock()
		cb.lastFailTime = time.Now()
		cb.mu.Unlock()

		if cb.failures.Load() >= int64(cb.maxFailures) {
			cb.state.Store(StateOpen)
		}
		return err
	}

	if state == StateHalfOpen {
		cb.failures.Store(0)
		cb.state.Store(StateClosed)
	}

	if state == StateClosed {
		cb.failures.Store(0)
	}

	return nil
}

func (cb *CircuitBreaker) State() int32 {
	return cb.state.Load()
}
