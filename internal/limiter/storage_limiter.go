package limiter

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
	chronostorage "github.com/SmitUplenchwar2687/Chrono/pkg/storage"
)

// StorageLimiter adapts a Storage backend to the Limiter interface.
type StorageLimiter struct {
	storage chronostorage.Storage
	clock   clock.Clock
	rate    int
	window  time.Duration
}

// NewStorageLimiter creates a Limiter backed by pkg/storage.
func NewStorageLimiter(storage chronostorage.Storage, rate int, window time.Duration, c clock.Clock) (*StorageLimiter, error) {
	if storage == nil {
		return nil, fmt.Errorf("storage is required")
	}
	if c == nil {
		return nil, fmt.Errorf("clock is required")
	}
	if rate <= 0 {
		return nil, fmt.Errorf("rate must be positive, got %d", rate)
	}
	if window <= 0 {
		return nil, fmt.Errorf("window must be positive, got %s", window)
	}

	return &StorageLimiter{
		storage: storage,
		clock:   c,
		rate:    rate,
		window:  window,
	}, nil
}

// Allow checks whether key is allowed, delegating state to the configured storage backend.
func (l *StorageLimiter) Allow(ctx context.Context, key string) Decision {
	allowed, remaining, resetAt, err := l.storage.CheckLimit(ctx, key, l.rate, l.window)
	if err != nil {
		now := l.clock.Now()
		log.Printf("storage limiter check failed for key %q: %v", key, err)
		return Decision{
			Allowed:   false,
			Remaining: 0,
			Limit:     l.rate,
			ResetAt:   now.Add(l.window),
			RetryAt:   now.Add(time.Second),
		}
	}

	d := Decision{
		Allowed:   allowed,
		Remaining: remaining,
		Limit:     l.rate,
		ResetAt:   resetAt,
	}
	if !allowed {
		d.RetryAt = resetAt
	}
	return d
}

// Close releases storage resources.
func (l *StorageLimiter) Close() error {
	return l.storage.Close()
}
