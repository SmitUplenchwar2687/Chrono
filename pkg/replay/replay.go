package replay

import (
	internalreplay "github.com/SmitUplenchwar2687/Chrono/internal/replay"
	"github.com/SmitUplenchwar2687/Chrono/pkg/clock"
	"github.com/SmitUplenchwar2687/Chrono/pkg/limiter"
)

// Filter defines criteria for selecting traffic records during replay.
type Filter = internalreplay.Filter

// Replayer replays recorded traffic through a rate limiter.
type Replayer = internalreplay.Replayer

// Result captures the outcome of replaying a single record.
type Result = internalreplay.Result

// Summary aggregates replay statistics.
type Summary = internalreplay.Summary

// KeySummary holds per-key replay stats.
type KeySummary = internalreplay.KeySummary

// New creates a new replayer.
func New(lim limiter.Limiter, vc *clock.VirtualClock, speed float64, filter *Filter) *Replayer {
	return internalreplay.New(lim, vc, speed, filter)
}
