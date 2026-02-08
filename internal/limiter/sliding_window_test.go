package limiter

import (
	"testing"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
)

func TestSlidingWindow_BasicAllow(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	sw := NewSlidingWindow(5, time.Minute, vc)

	d := sw.Allow(ctx, "user1")
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

func TestSlidingWindow_ExhaustLimit(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	sw := NewSlidingWindow(3, time.Minute, vc)

	for i := 0; i < 3; i++ {
		d := sw.Allow(ctx, "user1")
		if !d.Allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	d := sw.Allow(ctx, "user1")
	if d.Allowed {
		t.Error("4th request should be denied")
	}
	if d.Remaining != 0 {
		t.Errorf("Remaining = %d, want 0", d.Remaining)
	}
}

func TestSlidingWindow_SlidingBehavior(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	sw := NewSlidingWindow(3, time.Minute, vc)

	// Send 3 requests at t=0.
	for i := 0; i < 3; i++ {
		sw.Allow(ctx, "user1")
	}

	// Denied at t=0.
	d := sw.Allow(ctx, "user1")
	if d.Allowed {
		t.Fatal("should be denied at limit")
	}

	// Advance 30 seconds — still within the window for all 3 requests.
	vc.Advance(30 * time.Second)
	d = sw.Allow(ctx, "user1")
	if d.Allowed {
		t.Error("should still be denied at 30s")
	}

	// Advance to 61 seconds — all 3 original requests should have expired.
	vc.Advance(31 * time.Second)
	d = sw.Allow(ctx, "user1")
	if !d.Allowed {
		t.Error("should be allowed after window slides past original requests")
	}
}

func TestSlidingWindow_GradualExpiry(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	sw := NewSlidingWindow(3, time.Minute, vc)

	// Request at t=0.
	sw.Allow(ctx, "user1")

	// Request at t=20s.
	vc.Advance(20 * time.Second)
	sw.Allow(ctx, "user1")

	// Request at t=40s.
	vc.Advance(20 * time.Second)
	sw.Allow(ctx, "user1")

	// Denied at t=40s.
	d := sw.Allow(ctx, "user1")
	if d.Allowed {
		t.Fatal("should be denied at limit")
	}

	// At t=61s, the first request (t=0) expires — one slot opens.
	vc.Advance(21 * time.Second)
	d = sw.Allow(ctx, "user1")
	if !d.Allowed {
		t.Error("should be allowed after first request expires")
	}

	// Now 3 requests in window again (t=20, t=40, t=61). Denied.
	d = sw.Allow(ctx, "user1")
	if d.Allowed {
		t.Error("should be denied again after refilling")
	}
}

func TestSlidingWindow_RetryAt(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	sw := NewSlidingWindow(2, time.Minute, vc)

	sw.Allow(ctx, "user1")
	vc.Advance(10 * time.Second)
	sw.Allow(ctx, "user1")

	d := sw.Allow(ctx, "user1")
	if d.Allowed {
		t.Fatal("should be denied")
	}

	// RetryAt should be when the oldest entry (t=0) expires: t=0 + 1 min = t=60s.
	expectedRetry := epoch.Add(time.Minute)
	if !d.RetryAt.Equal(expectedRetry) {
		t.Errorf("RetryAt = %v, want %v", d.RetryAt, expectedRetry)
	}
}

func TestSlidingWindow_SeparateKeys(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	sw := NewSlidingWindow(1, time.Minute, vc)

	sw.Allow(ctx, "user1")
	d := sw.Allow(ctx, "user1")
	if d.Allowed {
		t.Error("user1 should be denied")
	}

	d = sw.Allow(ctx, "user2")
	if !d.Allowed {
		t.Error("user2 should be allowed (separate window)")
	}
}

func TestSlidingWindow_TimeTravel(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	sw := NewSlidingWindow(100, time.Hour, vc)

	for i := 0; i < 100; i++ {
		sw.Allow(ctx, "user1")
	}

	d := sw.Allow(ctx, "user1")
	if d.Allowed {
		t.Fatal("should be denied")
	}

	// TIME TRAVEL: fast-forward 1 hour.
	vc.Advance(time.Hour)

	d = sw.Allow(ctx, "user1")
	if !d.Allowed {
		t.Error("should be allowed after time travel of 1 hour")
	}
}

func TestSlidingWindow_ImplementsLimiter(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	var _ Limiter = NewSlidingWindow(10, time.Minute, vc)
}
