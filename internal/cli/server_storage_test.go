package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
	"github.com/SmitUplenchwar2687/Chrono/internal/config"
	"github.com/SmitUplenchwar2687/Chrono/internal/limiter"
)

func TestNormalizeRedisHostPort(t *testing.T) {
	host, port, err := normalizeRedisHostPort("localhost:6380", 6379)
	if err != nil {
		t.Fatalf("normalizeRedisHostPort() error = %v", err)
	}
	if host != "localhost" || port != 6380 {
		t.Fatalf("normalizeRedisHostPort() = %s:%d, want localhost:6380", host, port)
	}

	host, port, err = normalizeRedisHostPort("redis.internal", 6379)
	if err != nil {
		t.Fatalf("normalizeRedisHostPort() error = %v", err)
	}
	if host != "redis.internal" || port != 6379 {
		t.Fatalf("normalizeRedisHostPort() = %s:%d, want redis.internal:6379", host, port)
	}
}

func TestNormalizeRedisHostPort_Invalid(t *testing.T) {
	if _, _, err := normalizeRedisHostPort("", 6379); err == nil {
		t.Fatal("expected error for empty host")
	}
	if _, _, err := normalizeRedisHostPort("localhost", 0); err == nil {
		t.Fatal("expected error for non-positive port")
	}
}

func TestCreateServerLimiter_Memory(t *testing.T) {
	vc := clock.NewVirtualClock(time.Now())
	cfg := config.StorageConfig{
		Backend: "memory",
		Memory: config.StorageMemoryConfig{
			CleanupInterval: time.Minute,
		},
	}

	lim, err := createServerLimiter(limiter.AlgorithmTokenBucket, 5, time.Minute, 5, vc, cfg)
	if err != nil {
		t.Fatalf("createServerLimiter() error = %v", err)
	}
	defer func() {
		if c, ok := lim.(interface{ Close() error }); ok {
			_ = c.Close()
		}
	}()

	d := lim.Allow(context.Background(), "user-1")
	if !d.Allowed {
		t.Fatal("first request should be allowed")
	}
}

func TestCreateServerLimiter_RedisRequiresSlidingWindow(t *testing.T) {
	vc := clock.NewVirtualClock(time.Now())
	cfg := config.StorageConfig{
		Backend: "redis",
		Redis: config.StorageRedisConfig{
			Host: "localhost",
			Port: 6379,
		},
	}

	_, err := createServerLimiter(limiter.AlgorithmTokenBucket, 5, time.Minute, 5, vc, cfg)
	if err == nil {
		t.Fatal("expected error for redis + token_bucket")
	}
	if !strings.Contains(err.Error(), "unsupported with redis backend") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateServerLimiter_CRDTRequiresSlidingWindow(t *testing.T) {
	vc := clock.NewVirtualClock(time.Now())
	cfg := config.StorageConfig{
		Backend: "crdt",
		CRDT: config.StorageCRDTConfig{
			NodeID:   "node-1",
			BindAddr: ":8081",
		},
	}

	_, err := createServerLimiter(limiter.AlgorithmFixedWindow, 5, time.Minute, 0, vc, cfg)
	if err == nil {
		t.Fatal("expected error for crdt + fixed_window")
	}
	if !strings.Contains(err.Error(), "unsupported with crdt backend") {
		t.Fatalf("unexpected error: %v", err)
	}
}
