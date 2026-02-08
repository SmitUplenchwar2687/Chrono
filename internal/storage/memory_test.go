package storage

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
)

var (
	epoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ctx   = context.Background()
)

func newTestStorage() (*MemoryStorage, *clock.VirtualClock) {
	vc := clock.NewVirtualClock(epoch)
	s := NewMemoryStorage(vc)
	return s, vc
}

func TestMemoryStorage_SetGet(t *testing.T) {
	s, _ := newTestStorage()

	err := s.Set(ctx, "key1", []byte("hello"), 0)
	if err != nil {
		t.Fatal(err)
	}

	val, err := s.Get(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "hello" {
		t.Errorf("Get() = %q, want %q", val, "hello")
	}
}

func TestMemoryStorage_GetMissing(t *testing.T) {
	s, _ := newTestStorage()

	val, err := s.Get(ctx, "missing")
	if err != nil {
		t.Fatal(err)
	}
	if val != nil {
		t.Errorf("Get(missing) = %v, want nil", val)
	}
}

func TestMemoryStorage_Expiration(t *testing.T) {
	s, vc := newTestStorage()

	err := s.Set(ctx, "key1", []byte("value"), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	// Should exist before expiration.
	val, _ := s.Get(ctx, "key1")
	if val == nil {
		t.Fatal("key should exist before expiration")
	}

	// Advance past expiration.
	vc.Advance(11 * time.Second)

	val, _ = s.Get(ctx, "key1")
	if val != nil {
		t.Errorf("key should be expired, got %q", val)
	}
}

func TestMemoryStorage_ExpirationBoundary(t *testing.T) {
	s, vc := newTestStorage()

	err := s.Set(ctx, "key1", []byte("value"), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	// Advance exactly to expiration — should still be valid (not after).
	vc.Advance(10 * time.Second)
	val, _ := s.Get(ctx, "key1")
	if val != nil {
		t.Error("key should be expired at exact boundary (now > expiresAt)")
	}
}

func TestMemoryStorage_NoExpiration(t *testing.T) {
	s, vc := newTestStorage()

	err := s.Set(ctx, "key1", []byte("forever"), 0)
	if err != nil {
		t.Fatal(err)
	}

	// Advance a long time.
	vc.Advance(24 * 365 * time.Hour)

	val, _ := s.Get(ctx, "key1")
	if string(val) != "forever" {
		t.Errorf("key with no expiration should persist, got %q", val)
	}
}

func TestMemoryStorage_Overwrite(t *testing.T) {
	s, _ := newTestStorage()

	s.Set(ctx, "key1", []byte("v1"), 0)
	s.Set(ctx, "key1", []byte("v2"), 0)

	val, _ := s.Get(ctx, "key1")
	if string(val) != "v2" {
		t.Errorf("Get() after overwrite = %q, want %q", val, "v2")
	}
}

func TestMemoryStorage_Delete(t *testing.T) {
	s, _ := newTestStorage()

	s.Set(ctx, "key1", []byte("value"), 0)
	err := s.Delete(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}

	val, _ := s.Get(ctx, "key1")
	if val != nil {
		t.Error("key should be deleted")
	}
}

func TestMemoryStorage_DeleteMissing(t *testing.T) {
	s, _ := newTestStorage()

	err := s.Delete(ctx, "missing")
	if err != nil {
		t.Errorf("Delete(missing) should not error, got %v", err)
	}
}

func TestMemoryStorage_Increment(t *testing.T) {
	s, _ := newTestStorage()

	val, err := s.Increment(ctx, "counter", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if val != 1 {
		t.Errorf("first Increment = %d, want 1", val)
	}

	val, err = s.Increment(ctx, "counter", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if val != 2 {
		t.Errorf("second Increment = %d, want 2", val)
	}

	val, err = s.Increment(ctx, "counter", 5, 0)
	if err != nil {
		t.Fatal(err)
	}
	if val != 7 {
		t.Errorf("third Increment(5) = %d, want 7", val)
	}
}

func TestMemoryStorage_IncrementWithExpiration(t *testing.T) {
	s, vc := newTestStorage()

	s.Increment(ctx, "counter", 1, 10*time.Second)
	s.Increment(ctx, "counter", 1, 10*time.Second)

	// Advance past expiration.
	vc.Advance(11 * time.Second)

	// Should start fresh.
	val, _ := s.Increment(ctx, "counter", 1, 10*time.Second)
	if val != 1 {
		t.Errorf("Increment after expiration = %d, want 1", val)
	}
}

func TestMemoryStorage_Cleanup(t *testing.T) {
	s, vc := newTestStorage()

	s.Set(ctx, "expire1", []byte("v"), 5*time.Second)
	s.Set(ctx, "expire2", []byte("v"), 10*time.Second)
	s.Set(ctx, "persist", []byte("v"), 0)

	vc.Advance(7 * time.Second)
	s.Cleanup()

	if s.Len() != 2 {
		t.Errorf("Len() after cleanup = %d, want 2", s.Len())
	}

	vc.Advance(5 * time.Second)
	s.Cleanup()

	if s.Len() != 1 {
		t.Errorf("Len() after second cleanup = %d, want 1", s.Len())
	}
}

func TestMemoryStorage_GetReturnsCopy(t *testing.T) {
	s, _ := newTestStorage()

	s.Set(ctx, "key1", []byte("original"), 0)
	val, _ := s.Get(ctx, "key1")
	val[0] = 'X' // Mutate the returned slice.

	val2, _ := s.Get(ctx, "key1")
	if string(val2) != "original" {
		t.Errorf("Get() returned mutable reference, got %q", val2)
	}
}

func TestMemoryStorage_ConcurrentAccess(t *testing.T) {
	s, _ := newTestStorage()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Increment(ctx, "counter", 1, 0)
		}()
	}
	wg.Wait()

	val, _ := s.Increment(ctx, "counter", 0, 0)
	if val != 100 {
		t.Errorf("concurrent Increment result = %d, want 100", val)
	}
}

func TestMemoryStorage_ImplementsStorage(t *testing.T) {
	var _ Storage = NewMemoryStorage(clock.NewRealClock())
}
