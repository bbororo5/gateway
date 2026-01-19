package engine

import (
	"sync"
	"time"
)

// CircuitPolicy opens an upstream after consecutive failures. A zero threshold disables it.
type CircuitPolicy struct {
	Threshold int           `yaml:"failure_threshold"`
	Cooldown  time.Duration `yaml:"cooldown"`
}

type outcome uint8

const (
	neutral outcome = iota
	success
	failure
)

type circuit struct {
	policy     CircuitPolicy
	mu         sync.Mutex
	failures   int
	generation uint64
	retryAfter time.Time
	probing    bool
}

func (c *circuit) admit(now time.Time) (uint64, bool) {
	if c.policy.Threshold == 0 {
		return 0, true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.retryAfter.IsZero() {
		return c.generation, true
	}
	if now.Before(c.retryAfter) || c.probing {
		return 0, false
	}
	c.probing = true
	return c.generation, true
}

func (c *circuit) record(generation uint64, result outcome, now time.Time) {
	if c.policy.Threshold == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if generation != c.generation {
		return
	}
	if c.probing {
		c.probing = false
		switch result {
		case success:
			c.retryAfter = time.Time{}
			c.failures = 0
		case failure:
			c.retryAfter = now.Add(c.policy.Cooldown)
		}
		c.generation++
		return
	}
	switch result {
	case success:
		c.failures = 0
	case failure:
		c.failures++
		if c.failures >= c.policy.Threshold {
			c.retryAfter = now.Add(c.policy.Cooldown)
			c.generation++
		}
	}
}

type circuitState uint8

const (
	circuitClosed circuitState = iota
	circuitOpen
	circuitHalfOpen
)

func (c *circuit) state(now time.Time) circuitState {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case c.retryAfter.IsZero():
		return circuitClosed
	case now.Before(c.retryAfter):
		return circuitOpen
	default:
		return circuitHalfOpen
	}
}
