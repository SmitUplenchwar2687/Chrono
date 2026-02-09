package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewReplayCmd_LoadsConfigFile(t *testing.T) {
	trafficPath := writeReplayFixture(t)
	configPath := filepath.Join(t.TempDir(), "chrono.json")
	config := `{
  "limiter": {
    "algorithm": "fixed_window",
    "rate": 5,
    "window": "1m"
  },
  "storage": {
    "backend": "memory",
    "memory": {
      "cleanup_interval": "1m"
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"replay", "--file", trafficPath, "--config", configPath, "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("replay command with config failed: %v", err)
	}
}

func TestNewReplayCmd_RedisRequiresSlidingWindow(t *testing.T) {
	trafficPath := writeReplayFixture(t)

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"replay", "--file", trafficPath, "--storage", "redis", "--algorithm", "fixed_window"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for redis with fixed_window")
	}
}

func writeReplayFixture(t *testing.T) string {
	t.Helper()

	traffic := `[
  {
    "timestamp": "2024-01-01T00:00:00Z",
    "key": "user1",
    "endpoint": "GET /api/check/user1"
  }
]`
	path := filepath.Join(t.TempDir(), "traffic.json")
	if err := os.WriteFile(path, []byte(traffic), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
