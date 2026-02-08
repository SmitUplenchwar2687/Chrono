package storage

import (
	"context"
	"time"
)

// Storage abstracts the backend for rate limiter state.
// Implementations must be safe for concurrent use.
type Storage interface {
	// Get retrieves the stored value for a key.
	// Returns nil, nil if the key does not exist or has expired.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores a value for a key with an expiration duration.
	// If exp is 0, the key does not expire.
	Set(ctx context.Context, key string, value []byte, exp time.Duration) error

	// Increment atomically increments a counter for a key, returning the new value.
	// If the key does not exist, it is created with the value of delta.
	// exp sets the expiration for the key (only applied on creation).
	Increment(ctx context.Context, key string, delta int64, exp time.Duration) (int64, error)

	// Delete removes a key.
	Delete(ctx context.Context, key string) error
}
