package limiter

import (
	"time"

	internallimiter "github.com/SmitUplenchwar2687/Chrono/internal/limiter"
	"github.com/SmitUplenchwar2687/Chrono/pkg/clock"
)

// NewSlidingWindow creates a sliding window limiter.
func NewSlidingWindow(limit int, window time.Duration, c clock.Clock) *SlidingWindow {
	return internallimiter.NewSlidingWindow(limit, window, c)
}
