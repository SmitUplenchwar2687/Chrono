package limiter

import (
	"context"
	"testing"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/pkg/clock"
	chronostorage "github.com/SmitUplenchwar2687/Chrono/pkg/storage"
)

func TestStorageLimiterPublicAPI(t *testing.T) {
	vc := clock.NewVirtualClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	store, err := chronostorage.NewMemoryStorage(&chronostorage.MemoryConfig{
		CleanupInterval: time.Minute,
		Algorithm:       chronostorage.AlgorithmFixedWindow,
		Clock:           vc,
	})
	if err != nil {
		t.Fatalf("NewMemoryStorage() error = %v", err)
	}
	defer func() { _ = store.Close() }()

	lim, err := NewStorageLimiter(store, 2, time.Minute, vc)
	if err != nil {
		t.Fatalf("NewStorageLimiter() error = %v", err)
	}
	defer func() { _ = lim.Close() }()

	d1 := lim.Allow(context.Background(), "user-1")
	d2 := lim.Allow(context.Background(), "user-1")
	d3 := lim.Allow(context.Background(), "user-1")

	if !d1.Allowed || !d2.Allowed {
		t.Fatal("first two requests should be allowed")
	}
	if d3.Allowed {
		t.Fatal("third request should be denied")
	}
}
