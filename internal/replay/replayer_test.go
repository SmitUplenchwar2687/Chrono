package replay

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
	"github.com/SmitUplenchwar2687/Chrono/internal/limiter"
	"github.com/SmitUplenchwar2687/Chrono/internal/recorder"
)

var epoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

func makeRecords(count int, key string, interval time.Duration) []recorder.TrafficRecord {
	records := make([]recorder.TrafficRecord, count)
	for i := range records {
		records[i] = recorder.TrafficRecord{
			Timestamp: epoch.Add(time.Duration(i) * interval),
			Key:       key,
			Endpoint:  "GET /api/data",
		}
	}
	return records
}

func TestReplayer_BasicReplay(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(5, time.Minute, 5, vc)
	r := New(lim, vc, 0, &Filter{}) // speed=0 → instant

	r.LoadRecords(makeRecords(10, "user1", time.Second))

	var results []Result
	summary, err := r.Run(context.Background(), func(res Result) {
		results = append(results, res)
	})
	if err != nil {
		t.Fatal(err)
	}

	if summary.Replayed != 10 {
		t.Errorf("Replayed = %d, want 10", summary.Replayed)
	}
	if summary.Allowed != 5 {
		t.Errorf("Allowed = %d, want 5", summary.Allowed)
	}
	if summary.Denied != 5 {
		t.Errorf("Denied = %d, want 5", summary.Denied)
	}
	if len(results) != 10 {
		t.Errorf("got %d results, want 10", len(results))
	}
}

func TestReplayer_AdvancesClock(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	// 5 per minute — after 1 minute gap, tokens refill.
	lim := limiter.NewTokenBucket(5, time.Minute, 5, vc)

	// 5 records at t=0..4s (uses all tokens) + 5 records at t=61..65s (after refill).
	records := append(
		makeRecords(5, "user1", time.Second),
		makeRecords(5, "user1", time.Second)...,
	)
	// Shift the second batch to t=61s.
	for i := 5; i < 10; i++ {
		records[i].Timestamp = epoch.Add(61*time.Second + time.Duration(i-5)*time.Second)
	}

	r := New(lim, vc, 0, &Filter{})
	r.LoadRecords(records)

	summary, err := r.Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// All 10 should be allowed because the clock advances past the refill point.
	if summary.Allowed != 10 {
		t.Errorf("Allowed = %d, want 10 (clock should advance between batches)", summary.Allowed)
	}
}

func TestReplayer_Filter_Keys(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(100, time.Minute, 100, vc)

	records := append(
		makeRecords(5, "user1", time.Second),
		makeRecords(5, "user2", time.Second)...,
	)

	r := New(lim, vc, 0, &Filter{Keys: []string{"user1"}})
	r.LoadRecords(records)

	summary, err := r.Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	if summary.Filtered != 5 {
		t.Errorf("Filtered = %d, want 5 (only user1)", summary.Filtered)
	}
	if summary.Replayed != 5 {
		t.Errorf("Replayed = %d, want 5", summary.Replayed)
	}
}

func TestReplayer_Filter_Endpoints(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(100, time.Minute, 100, vc)

	records := []recorder.TrafficRecord{
		{Timestamp: epoch, Key: "u1", Endpoint: "GET /api/data"},
		{Timestamp: epoch.Add(time.Second), Key: "u1", Endpoint: "POST /api/data"},
		{Timestamp: epoch.Add(2 * time.Second), Key: "u1", Endpoint: "GET /health"},
	}

	r := New(lim, vc, 0, &Filter{Endpoints: []string{"/api/data"}})
	r.LoadRecords(records)

	summary, err := r.Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	if summary.Filtered != 2 {
		t.Errorf("Filtered = %d, want 2", summary.Filtered)
	}
}

func TestReplayer_Filter_TimeRange(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(100, time.Minute, 100, vc)

	records := makeRecords(10, "user1", time.Minute) // t=0, t=1m, ..., t=9m

	r := New(lim, vc, 0, &Filter{
		After:  epoch.Add(2 * time.Minute),
		Before: epoch.Add(6 * time.Minute),
	})
	r.LoadRecords(records)

	summary, err := r.Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Records at t=3m, t=4m, t=5m pass (after 2m and before 6m).
	if summary.Filtered != 3 {
		t.Errorf("Filtered = %d, want 3", summary.Filtered)
	}
}

func TestReplayer_Load_FromJSON(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(100, time.Minute, 100, vc)

	records := makeRecords(3, "user1", time.Second)
	data, _ := json.Marshal(records)

	r := New(lim, vc, 0, &Filter{})
	err := r.Load(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	summary, err := r.Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Replayed != 3 {
		t.Errorf("Replayed = %d, want 3", summary.Replayed)
	}
}

func TestReplayer_EmptyRecords(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(10, time.Minute, 10, vc)
	r := New(lim, vc, 0, &Filter{})

	_, err := r.Run(context.Background(), nil)
	if err == nil {
		t.Error("expected error for empty records")
	}
}

func TestReplayer_ContextCancellation(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(100, time.Minute, 100, vc)
	r := New(lim, vc, 0, &Filter{})
	r.LoadRecords(makeRecords(1000, "user1", time.Second))

	ctx, cancel := context.WithCancel(context.Background())
	count := 0
	summary, err := r.Run(ctx, func(res Result) {
		count++
		if count >= 5 {
			cancel()
		}
	})

	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if summary.Replayed < 5 {
		t.Errorf("should have replayed at least 5, got %d", summary.Replayed)
	}
}

func TestReplayer_PerKeySummary(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(3, time.Minute, 3, vc)

	records := append(
		makeRecords(5, "user1", time.Second),
		makeRecords(5, "user2", time.Second)...,
	)

	r := New(lim, vc, 0, &Filter{})
	r.LoadRecords(records)

	summary, err := r.Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	u1 := summary.PerKey["user1"]
	if u1.Allowed != 3 || u1.Denied != 2 {
		t.Errorf("user1: allowed=%d denied=%d, want 3/2", u1.Allowed, u1.Denied)
	}

	u2 := summary.PerKey["user2"]
	if u2.Allowed != 3 || u2.Denied != 2 {
		t.Errorf("user2: allowed=%d denied=%d, want 3/2", u2.Allowed, u2.Denied)
	}
}

func TestReplayer_SortsRecords(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(100, time.Minute, 100, vc)

	// Records in reverse order.
	records := []recorder.TrafficRecord{
		{Timestamp: epoch.Add(2 * time.Second), Key: "u1", Endpoint: "GET /c"},
		{Timestamp: epoch, Key: "u1", Endpoint: "GET /a"},
		{Timestamp: epoch.Add(time.Second), Key: "u1", Endpoint: "GET /b"},
	}

	r := New(lim, vc, 0, &Filter{})
	r.LoadRecords(records)

	var order []string
	r.Run(context.Background(), func(res Result) {
		order = append(order, res.Record.Endpoint)
	})

	if order[0] != "GET /a" || order[1] != "GET /b" || order[2] != "GET /c" {
		t.Errorf("records not sorted, got %v", order)
	}
}
