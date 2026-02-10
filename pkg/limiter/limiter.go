package limiter

import internallimiter "github.com/SmitUplenchwar2687/Chrono/internal/limiter"

// Algorithm identifies a rate limiting algorithm.
type Algorithm = internallimiter.Algorithm

const (
	AlgorithmTokenBucket   = internallimiter.AlgorithmTokenBucket
	AlgorithmSlidingWindow = internallimiter.AlgorithmSlidingWindow
	AlgorithmFixedWindow   = internallimiter.AlgorithmFixedWindow
)

// Limiter is the core rate limiting interface.
type Limiter = internallimiter.Limiter

// Decision captures the result of a rate limit check.
type Decision = internallimiter.Decision

// Config holds parameters for creating a limiter.
type Config = internallimiter.Config

// TokenBucket implements the token bucket rate limiting algorithm.
type TokenBucket = internallimiter.TokenBucket

// SlidingWindow implements the sliding window log rate limiting algorithm.
type SlidingWindow = internallimiter.SlidingWindow

// FixedWindow implements the fixed window counter rate limiting algorithm.
type FixedWindow = internallimiter.FixedWindow

// StorageLimiter adapts pkg/storage backends to the Limiter interface.
type StorageLimiter = internallimiter.StorageLimiter
