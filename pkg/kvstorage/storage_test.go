package kvstorage

import (
	"context"
	"testing"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/pkg/clock"
)

func TestMemoryStoragePublicAPI(t *testing.T) {
	vc := clock.NewVirtualClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	s := NewMemoryStorage(vc)

	ctx := context.Background()
	if err := s.Set(ctx, "k1", []byte("v1"), time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	got, err := s.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if string(got) != "v1" {
		t.Fatalf("Get() = %q, want %q", string(got), "v1")
	}

	n, err := s.Increment(ctx, "counter", 1, time.Minute)
	if err != nil {
		t.Fatalf("Increment() error = %v", err)
	}
	if n != 1 {
		t.Fatalf("Increment() = %d, want 1", n)
	}
}
