package storage

import (
	"context"
	"strconv"
	"testing"
	"time"

	testcontainers "github.com/testcontainers/testcontainers-go"
	rediscontainer "github.com/testcontainers/testcontainers-go/modules/redis"
)

func newRedisStorageForTest(t *testing.T) (*RedisStorage, func()) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx := context.Background()
	container, err := rediscontainer.Run(ctx, "redis:7.2-alpine")
	if err != nil {
		t.Skipf("redis container unavailable: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "6379/tcp")
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("container mapped port: %v", err)
	}

	p, err := strconv.Atoi(port.Port())
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("parse mapped port: %v", err)
	}

	store, err := NewRedisStorage(&RedisConfig{
		Host:        host,
		Port:        p,
		DB:          0,
		PoolSize:    20,
		MaxRetries:  3,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("NewRedisStorage() error: %v", err)
	}

	cleanup := func() {
		_ = store.Close()
		_ = container.Terminate(context.Background())
	}
	return store, cleanup
}
