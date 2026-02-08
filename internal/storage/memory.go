package storage

import (
	"context"
	"encoding/binary"
	"sync"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
)

// MemoryStorage is an in-memory storage backend backed by a map.
// It uses a Clock for expiration checks, enabling virtual-time testing.
// Thread-safe for concurrent use.
type MemoryStorage struct {
	mu    sync.RWMutex
	items map[string]memItem
	clock clock.Clock
}

type memItem struct {
	value     []byte
	expiresAt time.Time // zero value means no expiration
}

// NewMemoryStorage creates a new in-memory storage using the given clock.
func NewMemoryStorage(c clock.Clock) *MemoryStorage {
	return &MemoryStorage{
		items: make(map[string]memItem),
		clock: c,
	}
}

func (s *MemoryStorage) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, ok := s.items[key]
	if !ok {
		return nil, nil
	}
	if !item.expiresAt.IsZero() && !s.clock.Now().Before(item.expiresAt) {
		return nil, nil
	}
	// Return a copy to prevent mutation.
	val := make([]byte, len(item.value))
	copy(val, item.value)
	return val, nil
}

func (s *MemoryStorage) Set(_ context.Context, key string, value []byte, exp time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := memItem{
		value: make([]byte, len(value)),
	}
	copy(item.value, value)

	if exp > 0 {
		item.expiresAt = s.clock.Now().Add(exp)
	}
	s.items[key] = item
	return nil
}

func (s *MemoryStorage) Increment(_ context.Context, key string, delta int64, exp time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var current int64

	item, ok := s.items[key]
	if ok && (item.expiresAt.IsZero() || s.clock.Now().Before(item.expiresAt)) {
		current = int64(binary.BigEndian.Uint64(item.value))
	} else {
		// Key doesn't exist or expired — start fresh.
		item = memItem{}
		if exp > 0 {
			item.expiresAt = s.clock.Now().Add(exp)
		}
	}

	current += delta
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(current))
	item.value = buf
	s.items[key] = item

	return current, nil
}

func (s *MemoryStorage) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.items, key)
	return nil
}

// Cleanup removes all expired items. Call periodically for long-running sessions.
func (s *MemoryStorage) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clock.Now()
	for key, item := range s.items {
		if !item.expiresAt.IsZero() && !now.Before(item.expiresAt) {
			delete(s.items, key)
		}
	}
}

// Len returns the number of items (including expired ones not yet cleaned up).
func (s *MemoryStorage) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}
