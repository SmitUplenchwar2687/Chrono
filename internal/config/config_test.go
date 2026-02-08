package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/limiter"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Server.Addr != ":8080" {
		t.Errorf("default addr = %q, want %q", cfg.Server.Addr, ":8080")
	}
	if cfg.Limiter.Algorithm != limiter.AlgorithmTokenBucket {
		t.Errorf("default algorithm = %q, want %q", cfg.Limiter.Algorithm, limiter.AlgorithmTokenBucket)
	}
	if cfg.Limiter.Rate != 10 {
		t.Errorf("default rate = %d, want 10", cfg.Limiter.Rate)
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Errorf("default config should be valid, got %v", err)
	}
}

func TestValidate_AllAlgorithms(t *testing.T) {
	for _, algo := range []limiter.Algorithm{
		limiter.AlgorithmTokenBucket,
		limiter.AlgorithmSlidingWindow,
		limiter.AlgorithmFixedWindow,
	} {
		cfg := Default()
		cfg.Limiter.Algorithm = algo
		if err := cfg.Validate(); err != nil {
			t.Errorf("algorithm %q should be valid, got %v", algo, err)
		}
	}
}

func TestValidate_BadRate(t *testing.T) {
	cfg := Default()
	cfg.Limiter.Rate = 0
	if err := cfg.Validate(); err == nil {
		t.Error("rate=0 should be invalid")
	}

	cfg.Limiter.Rate = -1
	if err := cfg.Validate(); err == nil {
		t.Error("rate=-1 should be invalid")
	}
}

func TestValidate_BadWindow(t *testing.T) {
	cfg := Default()
	cfg.Limiter.Window = 0
	if err := cfg.Validate(); err == nil {
		t.Error("window=0 should be invalid")
	}

	cfg.Limiter.Window = -time.Second
	if err := cfg.Validate(); err == nil {
		t.Error("negative window should be invalid")
	}
}

func TestValidate_BadAlgorithm(t *testing.T) {
	cfg := Default()
	cfg.Limiter.Algorithm = "bogus"
	if err := cfg.Validate(); err == nil {
		t.Error("unknown algorithm should be invalid")
	}
}

func TestLoadFile_Full(t *testing.T) {
	content := `{
  "server": { "addr": ":9090" },
  "limiter": {
    "algorithm": "sliding_window",
    "rate": 100,
    "window": "30s",
    "burst": 50
  }
}`
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(content), 0o644)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Server.Addr != ":9090" {
		t.Errorf("addr = %q, want %q", cfg.Server.Addr, ":9090")
	}
	if cfg.Limiter.Algorithm != limiter.AlgorithmSlidingWindow {
		t.Errorf("algorithm = %q, want %q", cfg.Limiter.Algorithm, limiter.AlgorithmSlidingWindow)
	}
	if cfg.Limiter.Rate != 100 {
		t.Errorf("rate = %d, want 100", cfg.Limiter.Rate)
	}
	if cfg.Limiter.Window != 30*time.Second {
		t.Errorf("window = %v, want 30s", cfg.Limiter.Window)
	}
	if cfg.Limiter.Burst != 50 {
		t.Errorf("burst = %d, want 50", cfg.Limiter.Burst)
	}
}

func TestLoadFile_Partial(t *testing.T) {
	content := `{ "limiter": { "rate": 42 } }`
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(content), 0o644)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Rate was overridden.
	if cfg.Limiter.Rate != 42 {
		t.Errorf("rate = %d, want 42", cfg.Limiter.Rate)
	}
	// Everything else stays default.
	if cfg.Server.Addr != ":8080" {
		t.Errorf("addr should stay default, got %q", cfg.Server.Addr)
	}
	if cfg.Limiter.Algorithm != limiter.AlgorithmTokenBucket {
		t.Errorf("algorithm should stay default, got %q", cfg.Limiter.Algorithm)
	}
	if cfg.Limiter.Window != time.Minute {
		t.Errorf("window should stay default, got %v", cfg.Limiter.Window)
	}
}

func TestLoadFile_NotFound(t *testing.T) {
	_, err := LoadFile("/nonexistent/config.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadFile_BadJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte("{bad json}"), 0o644)

	_, err := LoadFile(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadFile_BadDuration(t *testing.T) {
	content := `{ "limiter": { "window": "not-a-duration" } }`
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(content), 0o644)

	_, err := LoadFile(path)
	if err == nil {
		t.Error("expected error for bad duration")
	}
}

func TestWriteExample(t *testing.T) {
	path := filepath.Join(t.TempDir(), "example.json")
	err := WriteExample(path)
	if err != nil {
		t.Fatal(err)
	}

	// Should be loadable.
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("example config should be valid, got %v", err)
	}
}
