package storage

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRedisStorage_CheckLimit_BasicAllowDeny(t *testing.T) {
	s, cleanup := newRedisStorageForTest(t)
	defer cleanup()

	limit := 3
	window := 2 * time.Second
	for i := 0; i < limit; i++ {
		allowed, remaining, _, err := s.CheckLimit(context.Background(), "u1", limit, window)
		if err != nil {
			t.Fatalf("CheckLimit() error = %v", err)
		}
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
		if remaining != limit-i-1 {
			t.Fatalf("remaining = %d, want %d", remaining, limit-i-1)
		}
	}

	allowed, remaining, resetAt, err := s.CheckLimit(context.Background(), "u1", limit, window)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if allowed {
		t.Fatal("request over limit should be denied")
	}
	if remaining != 0 {
		t.Fatalf("remaining = %d, want 0", remaining)
	}
	if !resetAt.After(time.Now()) {
		t.Fatalf("resetAt should be in the future, got %v", resetAt)
	}
}

func TestRedisStorage_CheckLimit_ResetAfterWindow(t *testing.T) {
	s, cleanup := newRedisStorageForTest(t)
	defer cleanup()

	limit := 1
	window := 500 * time.Millisecond

	allowed, _, _, err := s.CheckLimit(context.Background(), "u-reset", limit, window)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if !allowed {
		t.Fatal("first request should be allowed")
	}

	allowed, _, _, err = s.CheckLimit(context.Background(), "u-reset", limit, window)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if allowed {
		t.Fatal("second request should be denied")
	}

	time.Sleep(window + 200*time.Millisecond)
	allowed, _, _, err = s.CheckLimit(context.Background(), "u-reset", limit, window)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if !allowed {
		t.Fatal("request should be allowed after window")
	}
}

func TestRedisStorage_CheckLimit_SeparateKeys(t *testing.T) {
	s, cleanup := newRedisStorageForTest(t)
	defer cleanup()

	limit := 1
	window := time.Minute

	allowed, _, _, err := s.CheckLimit(context.Background(), "user-a", limit, window)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if !allowed {
		t.Fatal("user-a first request should be allowed")
	}

	allowed, _, _, err = s.CheckLimit(context.Background(), "user-a", limit, window)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if allowed {
		t.Fatal("user-a second request should be denied")
	}

	allowed, _, _, err = s.CheckLimit(context.Background(), "user-b", limit, window)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if !allowed {
		t.Fatal("user-b should be isolated and allowed")
	}
}

func TestRedisStorage_CheckLimit_Concurrent(t *testing.T) {
	s, cleanup := newRedisStorageForTest(t)
	defer cleanup()

	const (
		limit = 50
		total = 300
	)

	var allowedCount atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed, _, _, err := s.CheckLimit(context.Background(), "u-conc", limit, time.Minute)
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

	got := allowedCount.Load()
	if got > limit {
		t.Fatalf("allowed count = %d, should be <= %d", got, limit)
	}
}

func TestRedisStorage_NewRedisStorage_FailFastOnBadEndpoint(t *testing.T) {
	_, err := NewRedisStorage(&RedisConfig{
		Host:        "127.0.0.1",
		Port:        1,
		DB:          0,
		PoolSize:    1,
		MaxRetries:  1,
		DialTimeout: 100 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected constructor to fail for bad endpoint")
	}
}

func TestRedisStorage_NewRedisStorage_ClusterRequiresNodes(t *testing.T) {
	_, err := NewRedisStorage(&RedisConfig{
		Cluster: true,
	})
	if err == nil {
		t.Fatal("expected error when cluster=true and cluster_nodes is empty")
	}
}

func TestRedisStorage_Close_Idempotent(t *testing.T) {
	s, cleanup := newRedisStorageForTest(t)
	defer cleanup()

	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestRedisStorage_CheckLimit_Validation(t *testing.T) {
	s, cleanup := newRedisStorageForTest(t)
	defer cleanup()

	_, _, _, err := s.CheckLimit(context.Background(), "", 1, time.Second)
	if err == nil {
		t.Fatal("expected key validation error")
	}
	_, _, _, err = s.CheckLimit(context.Background(), "u1", 0, time.Second)
	if err == nil {
		t.Fatal("expected limit validation error")
	}
	_, _, _, err = s.CheckLimit(context.Background(), "u1", 1, 0)
	if err == nil {
		t.Fatal("expected window validation error")
	}
}
