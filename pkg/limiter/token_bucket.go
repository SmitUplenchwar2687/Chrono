package limiter

import (
	"time"

	"github.com/SmitUplenchwar2687/Chrono/pkg/clock"
	internallimiter "github.com/SmitUplenchwar2687/Chrono/internal/limiter"
)

// NewTokenBucket creates a token bucket limiter.
func NewTokenBucket(rate int, window time.Duration, burst int, c clock.Clock) *TokenBucket {
	return internallimiter.NewTokenBucket(rate, window, burst, c)
}
