package storage

import (
	"context"
	"testing"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/pkg/clock"
)

func TestMemoryStorageSetGet(t *testing.T) {
	s := NewMemoryStorage(clock.NewVirtualClock(time.Now()))
	err := s.Set(context.Background(), "k1", []byte("v1"), 0)
	if err != nil {
		t.Fatalf("Set() failed: %v", err)
	}
	got, err := s.Get(context.Background(), "k1")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if string(got) != "v1" {
		t.Fatalf("Get() = %q, want %q", string(got), "v1")
	}
}
