package limiter

import (
	"context"
	"sync"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
)

// SlidingWindow implements the sliding window log rate limiting algorithm.
//
// It stores the timestamp of each request and counts how many fall within the
// current window. This gives precise rate limiting without the boundary issues
// of fixed windows, at the cost of storing individual timestamps.
//
// Uses a Clock interface so it works with VirtualClock for time-travel testing.
type SlidingWindow struct {
	clock  clock.Clock
	limit  int
	window time.Duration
	mu     sync.Mutex
	logs   map[string][]time.Time
}

// NewSlidingWindow creates a sliding window limiter.
//   - limit: max requests allowed per window
//   - window: duration of the sliding window
//   - c: clock to use for time
func NewSlidingWindow(limit int, window time.Duration, c clock.Clock) *SlidingWindow {
	return &SlidingWindow{
		clock:  c,
		limit:  limit,
		window: window,
		logs:   make(map[string][]time.Time),
	}
}

func (sw *SlidingWindow) Allow(_ context.Context, key string) Decision {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := sw.clock.Now()
	windowStart := now.Add(-sw.window)

	// Prune expired entries.
	entries := sw.logs[key]
	pruned := entries[:0]
	for _, ts := range entries {
		if ts.After(windowStart) {
			pruned = append(pruned, ts)
		}
	}

	count := len(pruned)

	// Calculate when the window resets (oldest entry falls off).
	var resetAt time.Time
	if count > 0 {
		resetAt = pruned[0].Add(sw.window)
	} else {
		resetAt = now.Add(sw.window)
	}

	if count < sw.limit {
		pruned = append(pruned, now)
		sw.logs[key] = pruned
		return Decision{
			Allowed:   true,
			Remaining: sw.limit - count - 1,
			Limit:     sw.limit,
			ResetAt:   resetAt,
		}
	}

	// Denied — retry when the oldest entry expires.
	retryAt := pruned[0].Add(sw.window)

	sw.logs[key] = pruned
	return Decision{
		Allowed:   false,
		Remaining: 0,
		Limit:     sw.limit,
		ResetAt:   resetAt,
		RetryAt:   retryAt,
	}
}
