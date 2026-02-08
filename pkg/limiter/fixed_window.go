package limiter

import (
	"time"

	internallimiter "github.com/SmitUplenchwar2687/Chrono/internal/limiter"
	"github.com/SmitUplenchwar2687/Chrono/pkg/clock"
)

// NewFixedWindow creates a fixed window limiter.
func NewFixedWindow(limit int, window time.Duration, c clock.Clock) *FixedWindow {
	return internallimiter.NewFixedWindow(limit, window, c)
}
