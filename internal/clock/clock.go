package clock

import "time"

// Clock abstracts time so rate limiters work with both real and virtual time.
// All time-dependent code in Chrono uses this interface instead of time.Now().
type Clock interface {
	// Now returns the current time.
	Now() time.Time
	// Since returns the duration elapsed since t.
	Since(t time.Time) time.Duration
	// After returns a channel that receives the current time after duration d.
	After(d time.Duration) <-chan time.Time
}

// RealClock delegates to the standard time package.
type RealClock struct{}

func NewRealClock() *RealClock {
	return &RealClock{}
}

func (c *RealClock) Now() time.Time {
	return time.Now()
}

func (c *RealClock) Since(t time.Time) time.Duration {
	return time.Since(t)
}

func (c *RealClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}
