package limiter

import (
	"context"
	"time"
)

// Algorithm identifies a rate limiting algorithm.
type Algorithm string

const (
	AlgorithmTokenBucket   Algorithm = "token_bucket"
	AlgorithmSlidingWindow Algorithm = "sliding_window"
	AlgorithmFixedWindow   Algorithm = "fixed_window"
)

// Limiter is the core rate limiting interface.
// All algorithms implement this against a Clock.
type Limiter interface {
	// Allow checks if a request identified by key is allowed.
	Allow(ctx context.Context, key string) Decision
}

// Decision captures the result of a rate limit check.
type Decision struct {
	Allowed   bool      `json:"allowed"`
	Remaining int       `json:"remaining"` // Tokens/requests remaining after this check
	Limit     int       `json:"limit"`     // Max tokens/requests
	ResetAt   time.Time `json:"reset_at"`  // When the window/bucket fully resets
	RetryAt   time.Time `json:"retry_at"`  // Earliest time to retry (if denied)
}

// Config holds the parameters for creating a limiter.
type Config struct {
	Algorithm Algorithm     `json:"algorithm"`
	Rate      int           `json:"rate"`   // Requests allowed per window
	Window    time.Duration `json:"window"` // Window duration
	Burst     int           `json:"burst"`  // Max burst (token bucket only)
}
