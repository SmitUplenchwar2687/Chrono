package limiter

import (
	"testing"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
)

func TestFixedWindow_BasicAllow(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	fw := NewFixedWindow(5, time.Minute, vc)

	d := fw.Allow(ctx, "user1")
	if !d.Allowed {
		t.Error("first request should be allowed")
	}
	if d.Remaining != 4 {
		t.Errorf("Remaining = %d, want 4", d.Remaining)
	}
	if d.Limit != 5 {
		t.Errorf("Limit = %d, want 5", d.Limit)
	}
}

func TestFixedWindow_ExhaustLimit(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	fw := NewFixedWindow(3, time.Minute, vc)

	for i := 0; i < 3; i++ {
		d := fw.Allow(ctx, "user1")
		if !d.Allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	d := fw.Allow(ctx, "user1")
	if d.Allowed {
		t.Error("4th request should be denied")
	}
	if d.Remaining != 0 {
		t.Errorf("Remaining = %d, want 0", d.Remaining)
	}
}

func TestFixedWindow_ResetsAtWindowBoundary(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	fw := NewFixedWindow(3, time.Minute, vc)

	// Exhaust limit.
	for i := 0; i < 3; i++ {
		fw.Allow(ctx, "user1")
	}
	d := fw.Allow(ctx, "user1")
	if d.Allowed {
		t.Fatal("should be denied")
	}

	// Advance to next window.
	vc.Advance(time.Minute)

	d = fw.Allow(ctx, "user1")
	if !d.Allowed {
		t.Error("should be allowed in new window")
	}
	if d.Remaining != 2 {
		t.Errorf("Remaining = %d, want 2", d.Remaining)
	}
}

func TestFixedWindow_MidWindowRequests(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	fw := NewFixedWindow(5, time.Minute, vc)

	// 2 requests at t=0.
	fw.Allow(ctx, "user1")
	fw.Allow(ctx, "user1")

	// Advance 30s (still same window).
	vc.Advance(30 * time.Second)

	// 3 more requests — should hit limit.
	fw.Allow(ctx, "user1")
	fw.Allow(ctx, "user1")
	fw.Allow(ctx, "user1")

	d := fw.Allow(ctx, "user1")
	if d.Allowed {
		t.Error("should be denied — same window, 5 used")
	}
}

func TestFixedWindow_RetryAtIsNextWindow(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	fw := NewFixedWindow(1, time.Minute, vc)

	fw.Allow(ctx, "user1")
	d := fw.Allow(ctx, "user1")
	if d.Allowed {
		t.Fatal("should be denied")
	}

	// RetryAt should be the start of the next window.
	nextWindow := epoch.Add(time.Minute)
	if !d.RetryAt.Equal(nextWindow) {
		t.Errorf("RetryAt = %v, want %v", d.RetryAt, nextWindow)
	}
}

func TestFixedWindow_SeparateKeys(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	fw := NewFixedWindow(1, time.Minute, vc)

	fw.Allow(ctx, "user1")
	d := fw.Allow(ctx, "user1")
	if d.Allowed {
		t.Error("user1 should be denied")
	}

	d = fw.Allow(ctx, "user2")
	if !d.Allowed {
		t.Error("user2 should be allowed (separate counter)")
	}
}

func TestFixedWindow_TimeTravel(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	fw := NewFixedWindow(100, time.Hour, vc)

	for i := 0; i < 100; i++ {
		fw.Allow(ctx, "user1")
	}

	d := fw.Allow(ctx, "user1")
	if d.Allowed {
		t.Fatal("should be denied")
	}

	// TIME TRAVEL: jump ahead 1 hour.
	vc.Advance(time.Hour)

	d = fw.Allow(ctx, "user1")
	if !d.Allowed {
		t.Error("should be allowed after time travel of 1 hour")
	}
}

func TestFixedWindow_MultipleWindowsAdvance(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	fw := NewFixedWindow(2, time.Minute, vc)

	// Window 1: use both slots.
	fw.Allow(ctx, "user1")
	fw.Allow(ctx, "user1")

	// Skip 3 windows ahead.
	vc.Advance(3 * time.Minute)

	// Should have a fresh counter.
	d := fw.Allow(ctx, "user1")
	if !d.Allowed {
		t.Error("should be allowed after skipping multiple windows")
	}
	if d.Remaining != 1 {
		t.Errorf("Remaining = %d, want 1", d.Remaining)
	}
}

func TestFixedWindow_ImplementsLimiter(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	var _ Limiter = NewFixedWindow(10, time.Minute, vc)
}
