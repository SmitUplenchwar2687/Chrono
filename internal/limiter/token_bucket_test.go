package limiter

import (
	"context"
	"testing"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
)

var (
	epoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ctx   = context.Background()
)

func TestTokenBucket_BasicAllow(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	tb := NewTokenBucket(10, time.Minute, 10, vc)

	d := tb.Allow(ctx, "user1")
	if !d.Allowed {
		t.Error("first request should be allowed")
	}
	if d.Remaining != 9 {
		t.Errorf("Remaining = %d, want 9", d.Remaining)
	}
	if d.Limit != 10 {
		t.Errorf("Limit = %d, want 10", d.Limit)
	}
}

func TestTokenBucket_ExhaustTokens(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	tb := NewTokenBucket(5, time.Minute, 5, vc)

	for i := 0; i < 5; i++ {
		d := tb.Allow(ctx, "user1")
		if !d.Allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	d := tb.Allow(ctx, "user1")
	if d.Allowed {
		t.Error("6th request should be denied")
	}
	if d.Remaining != 0 {
		t.Errorf("Remaining = %d, want 0", d.Remaining)
	}
	if d.RetryAt.IsZero() {
		t.Error("RetryAt should be set when denied")
	}
}

func TestTokenBucket_RefillOverTime(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	// 10 per minute = 1 per 6 seconds
	tb := NewTokenBucket(10, time.Minute, 10, vc)

	// Exhaust all tokens.
	for i := 0; i < 10; i++ {
		tb.Allow(ctx, "user1")
	}

	d := tb.Allow(ctx, "user1")
	if d.Allowed {
		t.Fatal("should be denied after exhausting tokens")
	}

	// Advance 6 seconds — should get 1 token back.
	vc.Advance(6 * time.Second)
	d = tb.Allow(ctx, "user1")
	if !d.Allowed {
		t.Error("should be allowed after 6 second refill")
	}
}

func TestTokenBucket_FullRefill(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	tb := NewTokenBucket(10, time.Minute, 10, vc)

	// Exhaust all tokens.
	for i := 0; i < 10; i++ {
		tb.Allow(ctx, "user1")
	}

	// Advance full window — all tokens should be back.
	vc.Advance(time.Minute)

	for i := 0; i < 10; i++ {
		d := tb.Allow(ctx, "user1")
		if !d.Allowed {
			t.Errorf("request %d should be allowed after full refill", i+1)
		}
	}
}

func TestTokenBucket_TokensCappedAtCapacity(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	tb := NewTokenBucket(10, time.Minute, 10, vc)

	// Wait 10 minutes — tokens should still cap at 10, not 100.
	vc.Advance(10 * time.Minute)

	count := 0
	for {
		d := tb.Allow(ctx, "user1")
		if !d.Allowed {
			break
		}
		count++
		if count > 20 {
			t.Fatal("too many allowed requests, tokens not capped")
		}
	}

	if count != 10 {
		t.Errorf("allowed %d requests, want 10 (capacity)", count)
	}
}

func TestTokenBucket_BurstExceedsRate(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	// Rate of 5/min but burst of 10.
	tb := NewTokenBucket(5, time.Minute, 10, vc)

	count := 0
	for {
		d := tb.Allow(ctx, "user1")
		if !d.Allowed {
			break
		}
		count++
	}

	if count != 10 {
		t.Errorf("burst allowed %d requests, want 10", count)
	}
}

func TestTokenBucket_SeparateKeys(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	tb := NewTokenBucket(2, time.Minute, 2, vc)

	tb.Allow(ctx, "user1")
	tb.Allow(ctx, "user1")
	d := tb.Allow(ctx, "user1")
	if d.Allowed {
		t.Error("user1 should be denied")
	}

	// user2 should still be allowed.
	d = tb.Allow(ctx, "user2")
	if !d.Allowed {
		t.Error("user2 should be allowed (separate bucket)")
	}
}

func TestTokenBucket_TimeTravel(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	tb := NewTokenBucket(100, time.Hour, 100, vc)

	// Exhaust all 100 tokens.
	for i := 0; i < 100; i++ {
		tb.Allow(ctx, "user1")
	}

	d := tb.Allow(ctx, "user1")
	if d.Allowed {
		t.Fatal("should be denied")
	}

	// TIME TRAVEL: fast-forward 1 hour instantly.
	vc.Advance(time.Hour)

	d = tb.Allow(ctx, "user1")
	if !d.Allowed {
		t.Error("should be allowed after time travel of 1 hour")
	}
	if d.Remaining != 99 {
		t.Errorf("Remaining = %d, want 99", d.Remaining)
	}
}

func TestTokenBucket_RetryAtAccuracy(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	// 1 per second
	tb := NewTokenBucket(60, time.Minute, 1, vc)

	tb.Allow(ctx, "user1")
	d := tb.Allow(ctx, "user1")
	if d.Allowed {
		t.Fatal("should be denied")
	}

	// RetryAt should be ~1 second from now.
	retryIn := d.RetryAt.Sub(vc.Now())
	if retryIn < 900*time.Millisecond || retryIn > 1100*time.Millisecond {
		t.Errorf("RetryAt is %v from now, want ~1s", retryIn)
	}
}

func TestTokenBucket_ImplementsLimiter(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	var _ Limiter = NewTokenBucket(10, time.Minute, 10, vc)
}
