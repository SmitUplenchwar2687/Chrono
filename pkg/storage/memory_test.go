package storage

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/pkg/clock"
)

func TestMemoryStorage_FixedWindow(t *testing.T) {
	vc := clock.NewVirtualClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	s, err := NewMemoryStorage(&MemoryConfig{
		Algorithm:       AlgorithmFixedWindow,
		CleanupInterval: time.Hour,
		Clock:           vc,
	})
	if err != nil {
		t.Fatalf("NewMemoryStorage() error = %v", err)
	}
	defer s.Close()

	for i := 0; i < 3; i++ {
		allowed, remaining, _, err := s.CheckLimit(context.Background(), "user1", 3, time.Minute)
		if err != nil {
			t.Fatalf("CheckLimit() error = %v", err)
		}
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
		if remaining != 2-i {
			t.Fatalf("remaining = %d, want %d", remaining, 2-i)
		}
	}

	allowed, remaining, _, err := s.CheckLimit(context.Background(), "user1", 3, time.Minute)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if allowed || remaining != 0 {
		t.Fatalf("expected deny with remaining=0, got allowed=%v remaining=%d", allowed, remaining)
	}

	vc.Advance(time.Minute)
	allowed, _, _, err = s.CheckLimit(context.Background(), "user1", 3, time.Minute)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if !allowed {
		t.Fatal("should allow after window reset")
	}
}

func TestMemoryStorage_SlidingWindow(t *testing.T) {
	vc := clock.NewVirtualClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	s, err := NewMemoryStorage(&MemoryConfig{
		Algorithm:       AlgorithmSlidingWindow,
		CleanupInterval: time.Hour,
		Clock:           vc,
	})
	if err != nil {
		t.Fatalf("NewMemoryStorage() error = %v", err)
	}
	defer s.Close()

	// Limit 2/minute.
	for i := 0; i < 2; i++ {
		allowed, _, _, err := s.CheckLimit(context.Background(), "user1", 2, time.Minute)
		if err != nil {
			t.Fatalf("CheckLimit() error = %v", err)
		}
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	allowed, _, resetAt, err := s.CheckLimit(context.Background(), "user1", 2, time.Minute)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if allowed {
		t.Fatal("third request should be denied")
	}
	if !resetAt.After(vc.Now()) {
		t.Fatal("resetAt should be in the future when denied")
	}

	vc.Advance(time.Minute + time.Second)
	allowed, _, _, err = s.CheckLimit(context.Background(), "user1", 2, time.Minute)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if !allowed {
		t.Fatal("request should be allowed after entries expire")
	}
}

func TestMemoryStorage_TokenBucket(t *testing.T) {
	vc := clock.NewVirtualClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	s, err := NewMemoryStorage(&MemoryConfig{
		Algorithm:       AlgorithmTokenBucket,
		CleanupInterval: time.Hour,
		Clock:           vc,
	})
	if err != nil {
		t.Fatalf("NewMemoryStorage() error = %v", err)
	}
	defer s.Close()

	for i := 0; i < 2; i++ {
		allowed, _, _, err := s.CheckLimit(context.Background(), "user1", 2, time.Minute)
		if err != nil {
			t.Fatalf("CheckLimit() error = %v", err)
		}
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	allowed, _, _, err := s.CheckLimit(context.Background(), "user1", 2, time.Minute)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if allowed {
		t.Fatal("third request should be denied")
	}

	vc.Advance(30 * time.Second) // 1 token refilled at 2/minute
	allowed, _, _, err = s.CheckLimit(context.Background(), "user1", 2, time.Minute)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if !allowed {
		t.Fatal("request should be allowed after refill")
	}
}

func TestMemoryStorage_ConcurrentCheckLimit(t *testing.T) {
	s, err := NewMemoryStorage(&MemoryConfig{
		Algorithm:       AlgorithmFixedWindow,
		CleanupInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewMemoryStorage() error = %v", err)
	}
	defer s.Close()

	const (
		limit = 100
		total = 500
	)

	var allowedCount atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed, _, _, err := s.CheckLimit(context.Background(), "same-key", limit, time.Minute)
			if err != nil {
				t.Errorf("CheckLimit() error = %v", err)
				return
			}
			if allowed {
				allowedCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := int(allowedCount.Load()); got != limit {
		t.Fatalf("allowed count = %d, want %d", got, limit)
	}
}

func TestMemoryStorage_CloseIsIdempotent(t *testing.T) {
	s, err := NewMemoryStorage(nil)
	if err != nil {
		t.Fatalf("NewMemoryStorage() error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}
