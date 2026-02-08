package limiter

import (
	"context"
	"sync"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
)

// TokenBucket implements the token bucket rate limiting algorithm.
//
// Tokens are added at a constant rate (rate tokens per window).
// Each request consumes one token. If no tokens are available,
// the request is denied. Burst allows short spikes above the steady rate.
//
// Uses a Clock interface so it works with VirtualClock for time-travel testing.
type TokenBucket struct {
	clock    clock.Clock
	rate     float64 // tokens per second
	capacity int     // max tokens (burst)
	mu       sync.Mutex
	buckets  map[string]*bucket
}

type bucket struct {
	tokens   float64
	lastFill time.Time
}

// NewTokenBucket creates a token bucket limiter.
//   - rate: number of requests allowed per window
//   - window: duration of the rate window
//   - burst: maximum tokens that can accumulate (0 means burst = rate)
//   - c: clock to use for time
func NewTokenBucket(rate int, window time.Duration, burst int, c clock.Clock) *TokenBucket {
	if burst <= 0 {
		burst = rate
	}
	tokensPerSec := float64(rate) / window.Seconds()
	return &TokenBucket{
		clock:    c,
		rate:     tokensPerSec,
		capacity: burst,
		buckets:  make(map[string]*bucket),
	}
}

func (tb *TokenBucket) Allow(_ context.Context, key string) Decision {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := tb.clock.Now()

	b, ok := tb.buckets[key]
	if !ok {
		b = &bucket{
			tokens:   float64(tb.capacity),
			lastFill: now,
		}
		tb.buckets[key] = b
	}

	// Refill tokens based on elapsed time.
	elapsed := now.Sub(b.lastFill).Seconds()
	b.tokens += elapsed * tb.rate
	if b.tokens > float64(tb.capacity) {
		b.tokens = float64(tb.capacity)
	}
	b.lastFill = now

	// Calculate when bucket fully refills for ResetAt.
	deficit := float64(tb.capacity) - b.tokens
	var resetAt time.Time
	if deficit > 0 {
		resetAt = now.Add(time.Duration(deficit / tb.rate * float64(time.Second)))
	} else {
		resetAt = now
	}

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return Decision{
			Allowed:   true,
			Remaining: int(b.tokens),
			Limit:     tb.capacity,
			ResetAt:   resetAt,
		}
	}

	// Denied — calculate when next token arrives.
	needed := 1.0 - b.tokens
	retryAfter := time.Duration(needed / tb.rate * float64(time.Second))

	return Decision{
		Allowed:   false,
		Remaining: 0,
		Limit:     tb.capacity,
		ResetAt:   resetAt,
		RetryAt:   now.Add(retryAfter),
	}
}
