package clock

import (
	"testing"
	"time"
)

func TestClockImplementations(t *testing.T) {
	var _ Clock = NewRealClock()
	var _ Clock = NewVirtualClock(time.Now())
}

func TestVirtualClockAdvance(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	vc := NewVirtualClock(start)
	vc.Advance(time.Minute)

	if got := vc.Now(); !got.Equal(start.Add(time.Minute)) {
		t.Fatalf("Now() = %v, want %v", got, start.Add(time.Minute))
	}
}
