package config

import (
	"fmt"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/limiter"
)

// Config is the top-level configuration for a Chrono session.
type Config struct {
	Server  ServerConfig  `json:"server"`
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
