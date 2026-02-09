package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
	"github.com/SmitUplenchwar2687/Chrono/internal/limiter"
)

var epoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

func TestRunTest_BasicTokenBucket(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(5, time.Minute, 5, vc)

	result := runTest(vc, lim, []string{"user1"}, 10, 0)

	if len(result.Batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(result.Batches))
	}

	s := result.Summary["user1"]
	if s.TotalRequests != 10 {
		t.Errorf("total requests = %d, want 10", s.TotalRequests)
	}
	if s.Allowed != 5 {
		t.Errorf("allowed = %d, want 5", s.Allowed)
	}
	if s.Denied != 5 {
		t.Errorf("denied = %d, want 5", s.Denied)
	}
}

func TestRunTest_WithFastForward(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(5, time.Minute, 5, vc)

	result := runTest(vc, lim, []string{"user1"}, 8, time.Minute)

	if len(result.Batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(result.Batches))
	}
	if result.FastForward != "1m0s" {
		t.Errorf("fast_forward = %q, want %q", result.FastForward, "1m0s")
	}

	// Batch 1: 5 allowed, 3 denied.
	s := result.Summary["user1"]

	// Batch 2 after fast-forward: tokens should be refilled.
	// Total: 5+5 = 10 allowed, 3+3 = 6 denied.
	if s.Allowed != 10 {
		t.Errorf("total allowed = %d, want 10", s.Allowed)
	}
	if s.Denied != 6 {
		t.Errorf("total denied = %d, want 6", s.Denied)
	}
}

func TestRunTest_MultipleKeys(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewTokenBucket(3, time.Minute, 3, vc)

	result := runTest(vc, lim, []string{"user1", "user2"}, 5, 0)

	for _, key := range []string{"user1", "user2"} {
		s := result.Summary[key]
		if s.TotalRequests != 5 {
			t.Errorf("%s: total = %d, want 5", key, s.TotalRequests)
		}
		if s.Allowed != 3 {
			t.Errorf("%s: allowed = %d, want 3", key, s.Allowed)
		}
		if s.Denied != 2 {
			t.Errorf("%s: denied = %d, want 2", key, s.Denied)
		}
	}
}

func TestRunTest_SlidingWindow(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewSlidingWindow(5, time.Minute, vc)

	result := runTest(vc, lim, []string{"user1"}, 8, time.Minute)

	s := result.Summary["user1"]
	if s.Allowed != 10 {
		t.Errorf("allowed = %d, want 10", s.Allowed)
	}
}

func TestRunTest_FixedWindow(t *testing.T) {
	vc := clock.NewVirtualClock(epoch)
	lim := limiter.NewFixedWindow(5, time.Minute, vc)

	result := runTest(vc, lim, []string{"user1"}, 8, time.Minute)

	s := result.Summary["user1"]
	if s.Allowed != 10 {
		t.Errorf("allowed = %d, want 10", s.Allowed)
	}
}

func TestNewTestCmd_ExecutesWithDefaults(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"test", "--requests", "5", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("test command failed: %v", err)
	}
}

func TestNewTestCmd_AllAlgorithms(t *testing.T) {
	for _, algo := range []string{"token_bucket", "sliding_window", "fixed_window"} {
		t.Run(algo, func(t *testing.T) {
			cmd := NewRootCmd()
			cmd.SetArgs([]string{"test", "--algorithm", algo, "--requests", "3", "--json"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("test command with %s failed: %v", algo, err)
			}
		})
	}
}

func TestNewTestCmd_WithFastForward(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"test", "--requests", "5", "--fast-forward", "1h", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("test command with fast-forward failed: %v", err)
	}
}

func TestNewTestCmd_InvalidAlgorithm(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"test", "--algorithm", "bogus"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for invalid algorithm")
	}
}

func TestNewTestCmd_RedisRequiresSlidingWindow(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"test", "--storage", "redis", "--algorithm", "token_bucket", "--requests", "1", "--json"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for redis with token_bucket")
	}
}

func TestNewTestCmd_LoadsConfigFile(t *testing.T) {
	content := `{
  "limiter": {
    "algorithm": "fixed_window",
    "rate": 2,
    "window": "1m"
  },
  "storage": {
    "backend": "memory",
    "memory": {
      "cleanup_interval": "1m"
    }
  }
}`
	path := filepath.Join(t.TempDir(), "chrono.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"test", "--config", path, "--requests", "2", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("test command with config failed: %v", err)
	}
}
