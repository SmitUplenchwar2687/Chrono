package config

import internalconfig "github.com/SmitUplenchwar2687/Chrono/internal/config"

// Config is the top-level configuration for a Chrono session.
type Config = internalconfig.Config

// ServerConfig holds HTTP server settings.
type ServerConfig = internalconfig.ServerConfig

// StorageConfig holds pluggable storage backend settings.
type StorageConfig = internalconfig.StorageConfig

// StorageMemoryConfig configures the in-memory storage backend.
type StorageMemoryConfig = internalconfig.StorageMemoryConfig

// StorageRedisConfig configures the Redis storage backend.
type StorageRedisConfig = internalconfig.StorageRedisConfig

// StorageCRDTConfig configures the CRDT storage backend.
type StorageCRDTConfig = internalconfig.StorageCRDTConfig

// Default returns a Config with sensible defaults.
func Default() Config {
	return internalconfig.Default()
}

// LoadFile reads a JSON config file and merges it with defaults.
func LoadFile(path string) (Config, error) {
	return internalconfig.LoadFile(path)
}

// WriteExample writes an example config file to the given path.
func WriteExample(path string) error {
	return internalconfig.WriteExample(path)
}
