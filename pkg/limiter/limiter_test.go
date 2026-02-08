package limiter

import (
	"context"
	"testing"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/pkg/clock"
)

func TestTokenBucketPublicAPI(t *testing.T) {
	vc := clock.NewVirtualClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	tb := NewTokenBucket(2, time.Minute, 2, vc)

	d1 := tb.Allow(context.Background(), "user1")
	d2 := tb.Allow(context.Background(), "user1")
	d3 := tb.Allow(context.Background(), "user1")

	if !d1.Allowed || !d2.Allowed {
		t.Fatal("first two requests should be allowed")
	}
	if d3.Allowed {
		t.Fatal("third request should be denied")
	}
}

func TestAlgorithmConstants(t *testing.T) {
	if AlgorithmTokenBucket == "" || AlgorithmSlidingWindow == "" || AlgorithmFixedWindow == "" {
		t.Fatal("algorithm constants should be non-empty")
	}
}
