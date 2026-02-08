package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/limiter"
)

// Config is the top-level configuration for a Chrono session.
type Config struct {
	Server  ServerConfig   `json:"server"`
	Limiter limiter.Config `json:"limiter"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Addr string `json:"addr"`
}

// Default returns a Config with sensible defaults.
func Default() Config {
	return Config{
		Server: ServerConfig{
			Addr: ":8080",
		},
		Limiter: limiter.Config{
			Algorithm: limiter.AlgorithmTokenBucket,
			Rate:      10,
			Window:    time.Minute,
			Burst:     10,
		},
	}
}

// Validate checks that the config is valid.
func (c Config) Validate() error {
	if c.Limiter.Rate <= 0 {
		return fmt.Errorf("rate must be positive, got %d", c.Limiter.Rate)
	}
	if c.Limiter.Window <= 0 {
		return fmt.Errorf("window must be positive, got %s", c.Limiter.Window)
	}
	switch c.Limiter.Algorithm {
	case limiter.AlgorithmTokenBucket, limiter.AlgorithmSlidingWindow, limiter.AlgorithmFixedWindow:
	default:
		return fmt.Errorf("unknown algorithm %q, must be one of: token_bucket, sliding_window, fixed_window", c.Limiter.Algorithm)
	}
	return nil
}

// LoadFile reads a JSON config file and merges it with defaults.
// Fields not specified in the file retain their default values.
func LoadFile(path string) (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("reading config file: %w", err)
	}

	// Use a raw intermediate struct to handle duration parsing.
	var raw rawConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return cfg, fmt.Errorf("parsing config file: %w", err)
	}

	if raw.Server.Addr != "" {
		cfg.Server.Addr = raw.Server.Addr
	}
	if raw.Limiter.Algorithm != "" {
		cfg.Limiter.Algorithm = limiter.Algorithm(raw.Limiter.Algorithm)
	}
	if raw.Limiter.Rate > 0 {
		cfg.Limiter.Rate = raw.Limiter.Rate
	}
	if raw.Limiter.Window != "" {
		d, err := time.ParseDuration(raw.Limiter.Window)
		if err != nil {
			return cfg, fmt.Errorf("parsing limiter.window: %w", err)
		}
		cfg.Limiter.Window = d
	}
	if raw.Limiter.Burst > 0 {
		cfg.Limiter.Burst = raw.Limiter.Burst
	}

	return cfg, nil
}

// rawConfig is the JSON-friendly representation with string durations.
type rawConfig struct {
	Server struct {
		Addr string `json:"addr"`
	} `json:"server"`
	Limiter struct {
		Algorithm string `json:"algorithm"`
		Rate      int    `json:"rate"`
		Window    string `json:"window"`
		Burst     int    `json:"burst"`
	} `json:"limiter"`
}

// WriteExample writes an example config file to the given path.
func WriteExample(path string) error {
	example := `{
  "server": {
    "addr": ":8080"
  },
  "limiter": {
    "algorithm": "token_bucket",
    "rate": 10,
    "window": "1m",
    "burst": 10
  }
}
`
	return os.WriteFile(path, []byte(example), 0o644)
}
