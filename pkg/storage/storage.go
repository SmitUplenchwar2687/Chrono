package storage

import (
	"context"
	"fmt"
	"time"

	chronoclock "github.com/SmitUplenchwar2687/Chrono/pkg/clock"
)

const (
	// BackendMemory selects the in-process memory backend.
	BackendMemory = "memory"
	// BackendRedis selects the Redis backend (phase 2).
	BackendRedis = "redis"
	// BackendCRDT selects the experimental CRDT backend (phase 2).
	BackendCRDT = "crdt"

	// AlgorithmTokenBucket selects token bucket limit checks.
	AlgorithmTokenBucket = "token_bucket"
	// AlgorithmSlidingWindow selects sliding window limit checks.
	AlgorithmSlidingWindow = "sliding_window"
	// AlgorithmFixedWindow selects fixed window limit checks.
	AlgorithmFixedWindow = "fixed_window"
)

// Storage abstracts rate-limit state backends.
type Storage interface {
	// CheckLimit checks if a request is allowed under a given limit/window.
	// It returns allow/deny, remaining requests, and when the allowance resets.
	CheckLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, int, time.Time, error)
	// Close releases backend resources.
	Close() error
}

// Config selects and configures the storage backend.
type Config struct {
	Backend string        `json:"backend" yaml:"backend"`
	Memory  *MemoryConfig `json:"memory,omitempty" yaml:"memory,omitempty"`
	Redis   *RedisConfig  `json:"redis,omitempty" yaml:"redis,omitempty"`
	CRDT    *CRDTConfig   `json:"crdt,omitempty" yaml:"crdt,omitempty"`
}

// RedisConfig is reserved for phase 2 Redis implementation.
type RedisConfig struct {
	Host         string            `json:"host" yaml:"host"`
	Port         int               `json:"port" yaml:"port"`
	Password     string            `json:"password" yaml:"password"`
	DB           int               `json:"db" yaml:"db"`
	Cluster      bool              `json:"cluster" yaml:"cluster"`
	ClusterNodes []string          `json:"cluster_nodes,omitempty" yaml:"cluster_nodes,omitempty"`
	PoolSize     int               `json:"pool_size" yaml:"pool_size"`
	MaxRetries   int               `json:"max_retries" yaml:"max_retries"`
	DialTimeout  time.Duration     `json:"dial_timeout" yaml:"dial_timeout"`
	Clock        chronoclock.Clock `json:"-" yaml:"-"`
}

// CRDTConfig is reserved for phase 2 experimental CRDT implementation.
type CRDTConfig struct {
	NodeID           string            `json:"node_id" yaml:"node_id"`
	BindAddr         string            `json:"bind_addr" yaml:"bind_addr"`
	Peers            []string          `json:"peers" yaml:"peers"`
	GossipInterval   time.Duration     `json:"gossip_interval" yaml:"gossip_interval"`
	PersistDir       string            `json:"persist_dir,omitempty" yaml:"persist_dir,omitempty"`
	SnapshotInterval time.Duration     `json:"snapshot_interval,omitempty" yaml:"snapshot_interval,omitempty"`
	WALSyncInterval  time.Duration     `json:"wal_sync_interval,omitempty" yaml:"wal_sync_interval,omitempty"`
	Clock            chronoclock.Clock `json:"-" yaml:"-"`
}

// NewStorage constructs the configured backend.
// Empty backend defaults to memory for backward compatibility.
func NewStorage(cfg Config) (Storage, error) {
	backend := cfg.Backend
	if backend == "" {
		backend = BackendMemory
	}

	switch backend {
	case BackendMemory:
		return NewMemoryStorage(cfg.Memory)
	case BackendRedis:
		return NewRedisStorage(cfg.Redis)
	case BackendCRDT:
		return NewCRDTStorage(cfg.CRDT)
	default:
		return nil, fmt.Errorf("unknown storage backend %q", backend)
	}
}
