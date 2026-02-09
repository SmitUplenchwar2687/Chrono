package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/SmitUplenchwar2687/Chrono/internal/limiter"
	chronostorage "github.com/SmitUplenchwar2687/Chrono/pkg/storage"
)

// Config is the top-level configuration for a Chrono session.
type Config struct {
	Server  ServerConfig   `json:"server"`
	Limiter limiter.Config `json:"limiter"`
	Storage StorageConfig  `json:"storage"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Addr string `json:"addr"`
}

// StorageConfig holds pluggable storage backend settings.
type StorageConfig struct {
	Backend string              `json:"backend"`
	Memory  StorageMemoryConfig `json:"memory"`
	Redis   StorageRedisConfig  `json:"redis"`
	CRDT    StorageCRDTConfig   `json:"crdt"`
}

// StorageMemoryConfig configures the in-memory storage backend.
type StorageMemoryConfig struct {
	CleanupInterval time.Duration `json:"cleanup_interval"`
	Algorithm       string        `json:"algorithm,omitempty"`
	Burst           int           `json:"burst,omitempty"`
}

// StorageRedisConfig configures the Redis storage backend.
type StorageRedisConfig struct {
	Host         string        `json:"host"`
	Port         int           `json:"port"`
	Password     string        `json:"password"`
	DB           int           `json:"db"`
	Cluster      bool          `json:"cluster"`
	ClusterNodes []string      `json:"cluster_nodes,omitempty"`
	PoolSize     int           `json:"pool_size"`
	MaxRetries   int           `json:"max_retries"`
	DialTimeout  time.Duration `json:"dial_timeout"`
}

// StorageCRDTConfig configures the experimental CRDT storage backend.
type StorageCRDTConfig struct {
	NodeID         string        `json:"node_id"`
	BindAddr       string        `json:"bind_addr"`
	Peers          []string      `json:"peers"`
	GossipInterval time.Duration `json:"gossip_interval"`
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
		Storage: StorageConfig{
			Backend: chronostorage.BackendMemory,
			Memory: StorageMemoryConfig{
				CleanupInterval: time.Minute,
			},
			Redis: StorageRedisConfig{
				Host: "localhost",
				Port: 6379,
			},
			CRDT: StorageCRDTConfig{
				BindAddr: ":8081",
			},
		},
	}
}

// Validate checks that the config is valid.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}

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

	backend := c.Storage.Backend
	if backend == "" {
		backend = chronostorage.BackendMemory
	}

	switch backend {
	case chronostorage.BackendMemory:
		if c.Storage.Memory.CleanupInterval <= 0 {
			return fmt.Errorf("storage.memory.cleanup_interval must be positive, got %s", c.Storage.Memory.CleanupInterval)
		}
	case chronostorage.BackendRedis:
		if c.Storage.Redis.Cluster {
			if len(c.Storage.Redis.ClusterNodes) == 0 {
				return fmt.Errorf("storage.redis.cluster_nodes is required when storage.redis.cluster=true")
			}
		} else {
			if c.Storage.Redis.Host == "" {
				return fmt.Errorf("storage.redis.host is required when storage.redis.cluster=false")
			}
			if c.Storage.Redis.Port <= 0 {
				return fmt.Errorf("storage.redis.port must be positive when storage.redis.cluster=false, got %d", c.Storage.Redis.Port)
			}
		}
	case chronostorage.BackendCRDT:
		if c.Storage.CRDT.NodeID == "" {
			return fmt.Errorf("storage.crdt.node_id is required when backend=crdt")
		}
		if c.Storage.CRDT.BindAddr == "" {
			return fmt.Errorf("storage.crdt.bind_addr is required when backend=crdt")
		}
	default:
		return fmt.Errorf("unknown storage backend %q, must be one of: memory, redis, crdt", backend)
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
	if raw.Storage.Backend != "" {
		cfg.Storage.Backend = raw.Storage.Backend
	}
	if raw.Storage.Memory.CleanupInterval != "" {
		d, err := time.ParseDuration(raw.Storage.Memory.CleanupInterval)
		if err != nil {
			return cfg, fmt.Errorf("parsing storage.memory.cleanup_interval: %w", err)
		}
		cfg.Storage.Memory.CleanupInterval = d
	}
	if raw.Storage.Memory.Algorithm != "" {
		cfg.Storage.Memory.Algorithm = raw.Storage.Memory.Algorithm
	}
	if raw.Storage.Memory.Burst > 0 {
		cfg.Storage.Memory.Burst = raw.Storage.Memory.Burst
	}

	if raw.Storage.Redis.Host != "" {
		cfg.Storage.Redis.Host = raw.Storage.Redis.Host
	}
	if raw.Storage.Redis.Port > 0 {
		cfg.Storage.Redis.Port = raw.Storage.Redis.Port
	}
	if raw.Storage.Redis.Password != "" {
		cfg.Storage.Redis.Password = raw.Storage.Redis.Password
	}
	if raw.Storage.Redis.DB != 0 {
		cfg.Storage.Redis.DB = raw.Storage.Redis.DB
	}
	cfg.Storage.Redis.Cluster = raw.Storage.Redis.Cluster
	if raw.Storage.Redis.ClusterNodes != nil {
		cfg.Storage.Redis.ClusterNodes = append([]string(nil), raw.Storage.Redis.ClusterNodes...)
	}
	if raw.Storage.Redis.PoolSize > 0 {
		cfg.Storage.Redis.PoolSize = raw.Storage.Redis.PoolSize
	}
	if raw.Storage.Redis.MaxRetries > 0 {
		cfg.Storage.Redis.MaxRetries = raw.Storage.Redis.MaxRetries
	}
	if raw.Storage.Redis.DialTimeout != "" {
		d, err := time.ParseDuration(raw.Storage.Redis.DialTimeout)
		if err != nil {
			return cfg, fmt.Errorf("parsing storage.redis.dial_timeout: %w", err)
		}
		cfg.Storage.Redis.DialTimeout = d
	}

	if raw.Storage.CRDT.NodeID != "" {
		cfg.Storage.CRDT.NodeID = raw.Storage.CRDT.NodeID
	}
	if raw.Storage.CRDT.BindAddr != "" {
		cfg.Storage.CRDT.BindAddr = raw.Storage.CRDT.BindAddr
	}
	if raw.Storage.CRDT.Peers != nil {
		cfg.Storage.CRDT.Peers = append([]string(nil), raw.Storage.CRDT.Peers...)
	}
	if raw.Storage.CRDT.GossipInterval != "" {
		d, err := time.ParseDuration(raw.Storage.CRDT.GossipInterval)
		if err != nil {
			return cfg, fmt.Errorf("parsing storage.crdt.gossip_interval: %w", err)
		}
		cfg.Storage.CRDT.GossipInterval = d
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
	Storage struct {
		Backend string `json:"backend"`
		Memory  struct {
			CleanupInterval string `json:"cleanup_interval"`
			Algorithm       string `json:"algorithm,omitempty"`
			Burst           int    `json:"burst,omitempty"`
		} `json:"memory"`
		Redis struct {
			Host         string   `json:"host"`
			Port         int      `json:"port"`
			Password     string   `json:"password"`
			DB           int      `json:"db"`
			Cluster      bool     `json:"cluster"`
			ClusterNodes []string `json:"cluster_nodes,omitempty"`
			PoolSize     int      `json:"pool_size"`
			MaxRetries   int      `json:"max_retries"`
			DialTimeout  string   `json:"dial_timeout"`
		} `json:"redis"`
		CRDT struct {
			NodeID         string   `json:"node_id"`
			BindAddr       string   `json:"bind_addr"`
			Peers          []string `json:"peers"`
			GossipInterval string   `json:"gossip_interval"`
		} `json:"crdt"`
	} `json:"storage"`
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
  },
  "storage": {
    "backend": "memory",
    "memory": {
      "cleanup_interval": "1m"
    }
  }
}
`
	return os.WriteFile(path, []byte(example), 0o644)
}
