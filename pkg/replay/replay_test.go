package replay

import (
	"context"
	"testing"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/pkg/clock"
	"github.com/SmitUplenchwar2687/Chrono/pkg/limiter"
	"github.com/SmitUplenchwar2687/Chrono/pkg/recorder"
)

func TestReplayBasic(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	vc := clock.NewVirtualClock(start)
	lim := limiter.NewTokenBucket(2, time.Minute, 2, vc)

	r := New(lim, vc, 0, &Filter{})
	r.LoadRecords([]recorder.TrafficRecord{
		{Timestamp: start, Key: "u1", Endpoint: "GET /api/profile"},
		{Timestamp: start.Add(time.Second), Key: "u1", Endpoint: "GET /api/profile"},
		{Timestamp: start.Add(2 * time.Second), Key: "u1", Endpoint: "GET /api/profile"},
	})

	summary, err := r.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}
	if summary.Replayed != 3 {
		t.Fatalf("Replayed = %d, want 3", summary.Replayed)
	}
	if summary.Allowed != 2 || summary.Denied != 1 {
		t.Fatalf("Allowed/Denied = %d/%d, want 2/1", summary.Allowed, summary.Denied)
	}
}
