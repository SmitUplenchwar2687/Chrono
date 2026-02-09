package storage

import (
	"context"
	"testing"
	"time"
)

type storageFactory struct {
	name string
	new  func(t *testing.T) (Storage, func())
}

func TestStorageContract(t *testing.T) {
	factories := []storageFactory{
		{
			name: "memory",
			new: func(t *testing.T) (Storage, func()) {
				t.Helper()
				s, err := NewMemoryStorage(&MemoryConfig{
					Algorithm:       AlgorithmFixedWindow,
					CleanupInterval: time.Minute,
				})
				if err != nil {
					t.Fatalf("NewMemoryStorage() error = %v", err)
				}
				return s, func() { _ = s.Close() }
			},
		},
		{
			name: "redis",
			new: func(t *testing.T) (Storage, func()) {
				t.Helper()
				s, cleanup := newRedisStorageForTest(t)
				return s, cleanup
			},
		},
	}

	for _, f := range factories {
		t.Run(f.name, func(t *testing.T) {
			store, cleanup := f.new(t)
			defer cleanup()

			contractAllowDeny(t, store)
			contractReset(t, store)
			contractKeyIsolation(t, store)
		})
	}
}

func contractAllowDeny(t *testing.T, s Storage) {
	t.Helper()
	limit := 2
	window := 300 * time.Millisecond
	key := "contract-allow-deny"

	for i := 0; i < limit; i++ {
		allowed, _, _, err := s.CheckLimit(context.Background(), key, limit, window)
		if err != nil {
			t.Fatalf("CheckLimit() error = %v", err)
		}
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	allowed, _, _, err := s.CheckLimit(context.Background(), key, limit, window)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if allowed {
		t.Fatal("request over limit should be denied")
	}
}

func contractReset(t *testing.T, s Storage) {
	t.Helper()
	limit := 1
	window := 250 * time.Millisecond
	key := "contract-reset"

	allowed, _, _, err := s.CheckLimit(context.Background(), key, limit, window)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if !allowed {
		t.Fatal("first request should be allowed")
	}

	allowed, _, _, err = s.CheckLimit(context.Background(), key, limit, window)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if allowed {
		t.Fatal("second request should be denied")
	}

	time.Sleep(window + 150*time.Millisecond)
	allowed, _, _, err = s.CheckLimit(context.Background(), key, limit, window)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if !allowed {
		t.Fatal("request should be allowed after window reset")
	}
}

func contractKeyIsolation(t *testing.T, s Storage) {
	t.Helper()
	limit := 1
	window := time.Second

	allowed, _, _, err := s.CheckLimit(context.Background(), "contract-key-a", limit, window)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if !allowed {
		t.Fatal("key-a first request should be allowed")
	}

	allowed, _, _, err = s.CheckLimit(context.Background(), "contract-key-a", limit, window)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if allowed {
		t.Fatal("key-a second request should be denied")
	}

	allowed, _, _, err = s.CheckLimit(context.Background(), "contract-key-b", limit, window)
	if err != nil {
		t.Fatalf("CheckLimit() error = %v", err)
	}
	if !allowed {
		t.Fatal("key-b should be isolated and allowed")
	}
}
