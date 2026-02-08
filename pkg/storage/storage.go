package storage

import (
	internalstorage "github.com/SmitUplenchwar2687/Chrono/internal/storage"
	"github.com/SmitUplenchwar2687/Chrono/pkg/clock"
)

// Storage abstracts the backend for rate limiter state.
type Storage = internalstorage.Storage

// MemoryStorage is an in-memory storage backend.
type MemoryStorage = internalstorage.MemoryStorage

// NewMemoryStorage creates a new in-memory storage using the given clock.
func NewMemoryStorage(c clock.Clock) *MemoryStorage {
	return internalstorage.NewMemoryStorage(c)
}
