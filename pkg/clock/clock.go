package clock

import (
	"time"

	internalclock "github.com/SmitUplenchwar2687/Chrono/internal/clock"
)

// Clock abstracts time so rate limiters work with both real and virtual time.
type Clock = internalclock.Clock

// RealClock delegates to the standard time package.
type RealClock = internalclock.RealClock

// VirtualClock is a controllable clock for time-travel testing.
type VirtualClock = internalclock.VirtualClock

// NewRealClock creates a real wall-clock implementation.
func NewRealClock() *RealClock {
	return internalclock.NewRealClock()
}

// NewVirtualClock creates a virtual clock starting at the given time.
func NewVirtualClock(start time.Time) *VirtualClock {
	return internalclock.NewVirtualClock(start)
}
