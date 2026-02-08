package limiter

import (
	"context"
	"sync"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
)

// FixedWindow implements the fixed window counter rate limiting algorithm.
//
// Time is divided into fixed windows (e.g., every minute). Each window has a
// counter. Requests increment the counter; if it exceeds the limit, the request
// is denied. The counter resets at the start of each new window.
//
// Simple and memory-efficient, but can allow up to 2x the rate at window boundaries.
//
// Uses a Clock interface so it works with VirtualClock for time-travel testing.
type FixedWindow struct {
	clock  clock.Clock
	limit  int
	window time.Duration
	mu     sync.Mutex
	counts map[string]*windowCounter
}

type windowCounter struct {
	count    int
	windowID int64 // which window this count belongs to
}

// NewFixedWindow creates a fixed window limiter.
//   - limit: max requests allowed per window
//   - window: duration of each fixed window
//   - c: clock to use for time
func NewFixedWindow(limit int, window time.Duration, c clock.Clock) *FixedWindow {
	return &FixedWindow{
		clock:  c,
		limit:  limit,
		window: window,
		counts: make(map[string]*windowCounter),
	}
}

// windowID returns a numeric identifier for the fixed window containing t.
func (fw *FixedWindow) windowID(t time.Time) int64 {
	return t.UnixNano() / int64(fw.window)
}

// windowStart returns the start time of the window containing t.
func (fw *FixedWindow) windowStart(t time.Time) time.Time {
	id := fw.windowID(t)
	return time.Unix(0, id*int64(fw.window))
}

func (fw *FixedWindow) Allow(_ context.Context, key string) Decision {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	now := fw.clock.Now()
	currentWindowID := fw.windowID(now)
	resetAt := fw.windowStart(now).Add(fw.window)

	wc, ok := fw.counts[key]
	if !ok || wc.windowID != currentWindowID {
		// New window — reset counter.
		wc = &windowCounter{
			count:    0,
			windowID: currentWindowID,
		}
		fw.counts[key] = wc
	}

	if wc.count < fw.limit {
		wc.count++
		return Decision{
			Allowed:   true,
			Remaining: fw.limit - wc.count,
			Limit:     fw.limit,
			ResetAt:   resetAt,
		}
	}

	return Decision{
		Allowed:   false,
		Remaining: 0,
		Limit:     fw.limit,
		ResetAt:   resetAt,
		RetryAt:   resetAt,
	}
}
