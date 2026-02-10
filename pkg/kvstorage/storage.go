package kvstorage

import (
	internalstorage "github.com/SmitUplenchwar2687/Chrono/internal/storage"
	"github.com/SmitUplenchwar2687/Chrono/pkg/clock"
)

// Storage is a generic key/value storage interface with expiration support.
type Storage = internalstorage.Storage

// MemoryStorage is an in-memory implementation of Storage.
type MemoryStorage = internalstorage.MemoryStorage

// NewMemoryStorage creates a memory-backed key/value storage.
func NewMemoryStorage(c clock.Clock) *MemoryStorage {
	return internalstorage.NewMemoryStorage(c)
}
