package clock

import (
	"sync"
	"time"
)

// VirtualClock is a controllable clock for time-travel testing.
// It allows advancing time instantly without waiting, making
// rate limiter tests deterministic and fast.
//
// Thread-safe for concurrent use.
type VirtualClock struct {
	mu      sync.RWMutex
	current time.Time
	waiters []waiter
}

type waiter struct {
	deadline time.Time
	ch       chan time.Time
}

// NewVirtualClock creates a VirtualClock starting at the given time.
func NewVirtualClock(start time.Time) *VirtualClock {
	return &VirtualClock{
		current: start,
	}
}

// Now returns the current virtual time.
func (c *VirtualClock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.current
}

// Since returns the virtual duration elapsed since t.
func (c *VirtualClock) Since(t time.Time) time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.current.Sub(t)
}

// After returns a channel that receives the virtual time once the clock
// has advanced past the current time plus d. The channel fires during
// Advance() or Set() calls when the deadline is reached.
func (c *VirtualClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	ch := make(chan time.Time, 1)
	deadline := c.current.Add(d)

	// If duration is zero or negative, fire immediately.
	if d <= 0 {
		ch <- c.current
		return ch
	}

	c.waiters = append(c.waiters, waiter{
		deadline: deadline,
		ch:       ch,
	})
	return ch
}

// Advance moves the virtual clock forward by the given duration.
// It fires any waiters whose deadlines have been reached.
// Panics if d is negative.
func (c *VirtualClock) Advance(d time.Duration) {
	if d < 0 {
		panic("clock: cannot advance by negative duration")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.current = c.current.Add(d)
	c.drainWaiters()
}

// Set sets the virtual clock to an exact time.
// It fires any waiters whose deadlines have been reached.
// Panics if t is before the current time.
func (c *VirtualClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if t.Before(c.current) {
		panic("clock: cannot set time to the past")
	}

	c.current = t
	c.drainWaiters()
}

// drainWaiters fires all waiters whose deadline is at or before the current time.
// Must be called with c.mu held.
func (c *VirtualClock) drainWaiters() {
	remaining := c.waiters[:0]
	for _, w := range c.waiters {
		if !w.deadline.After(c.current) {
			w.ch <- c.current
		} else {
			remaining = append(remaining, w)
		}
	}
	c.waiters = remaining
}
