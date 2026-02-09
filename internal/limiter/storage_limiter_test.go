package limiter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
)

type stubStorage struct {
	allowed   bool
	remaining int
	resetAt   time.Time
	err       error
	closed    bool
}

func (s *stubStorage) CheckLimit(_ context.Context, _ string, _ int, _ time.Duration) (bool, int, time.Time, error) {
	return s.allowed, s.remaining, s.resetAt, s.err
}

func (s *stubStorage) Close() error {
	s.closed = true
	return nil
}

func TestNewStorageLimiter_Validate(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)

	if _, err := NewStorageLimiter(nil, 1, time.Second, vc); err == nil {
		t.Fatal("expected error for nil storage")
	}

	if _, err := NewStorageLimiter(&stubStorage{}, 0, time.Second, vc); err == nil {
		t.Fatal("expected error for non-positive rate")
	}

	if _, err := NewStorageLimiter(&stubStorage{}, 1, 0, vc); err == nil {
		t.Fatal("expected error for non-positive window")
	}

	if _, err := NewStorageLimiter(&stubStorage{}, 1, time.Second, nil); err == nil {
		t.Fatal("expected error for nil clock")
	}
}

func TestStorageLimiter_Allow(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	st := &stubStorage{
		allowed:   true,
		remaining: 7,
		resetAt:   epoch.Add(time.Minute),
	}
	lim, err := NewStorageLimiter(st, 10, time.Minute, vc)
	if err != nil {
		t.Fatalf("NewStorageLimiter() error = %v", err)
	}

	d := lim.Allow(context.Background(), "user-1")
	if !d.Allowed {
		t.Fatal("decision should be allowed")
	}
	if d.Remaining != 7 {
		t.Fatalf("remaining = %d, want 7", d.Remaining)
	}
	if d.Limit != 10 {
		t.Fatalf("limit = %d, want 10", d.Limit)
	}
	if !d.ResetAt.Equal(st.resetAt) {
		t.Fatalf("resetAt = %v, want %v", d.ResetAt, st.resetAt)
	}
	if !d.RetryAt.IsZero() {
		t.Fatalf("retryAt should be zero when allowed, got %v", d.RetryAt)
	}
}

func TestStorageLimiter_DenySetsRetryAt(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	st := &stubStorage{
		allowed: false,
		resetAt: epoch.Add(30 * time.Second),
	}
	lim, err := NewStorageLimiter(st, 10, time.Minute, vc)
	if err != nil {
		t.Fatalf("NewStorageLimiter() error = %v", err)
	}

	d := lim.Allow(context.Background(), "user-1")
	if d.Allowed {
		t.Fatal("decision should be denied")
	}
	if !d.RetryAt.Equal(st.resetAt) {
		t.Fatalf("retryAt = %v, want %v", d.RetryAt, st.resetAt)
	}
}

func TestStorageLimiter_ErrorFailsClosed(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	st := &stubStorage{err: errors.New("boom")}
	lim, err := NewStorageLimiter(st, 10, time.Minute, vc)
	if err != nil {
		t.Fatalf("NewStorageLimiter() error = %v", err)
	}

	d := lim.Allow(context.Background(), "user-1")
	if d.Allowed {
		t.Fatal("error path should deny request")
	}
	if d.Limit != 10 {
		t.Fatalf("limit = %d, want 10", d.Limit)
	}
	if d.RetryAt.IsZero() {
		t.Fatal("retryAt should be set on error path")
	}
}

func TestStorageLimiter_Close(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	st := &stubStorage{}
	lim, err := NewStorageLimiter(st, 10, time.Minute, vc)
	if err != nil {
		t.Fatalf("NewStorageLimiter() error = %v", err)
	}

	if err := lim.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !st.closed {
		t.Fatal("expected storage close to be called")
	}
}
