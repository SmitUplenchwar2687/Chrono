package cli

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SmitUplenchwar2687/Chrono/internal/config"
	chronostorage "github.com/SmitUplenchwar2687/Chrono/pkg/storage"
)

type storageOptions struct {
	backend               string
	memoryCleanupInterval time.Duration
	redisHost             string
	redisPort             int
	redisPassword         string
	redisDB               int
	redisCluster          bool
	redisClusterNodes     []string
	redisPoolSize         int
	redisMaxRetries       int
	redisDialTimeout      time.Duration
	crdtNodeID            string
	crdtBindAddr          string
	crdtPeers             []string
	crdtGossipInterval    time.Duration
	crdtPersistDir        string
	crdtSnapshotInterval  time.Duration
}

func defaultStorageOptions() storageOptions {
	return storageOptions{
		backend:               chronostorage.BackendMemory,
		memoryCleanupInterval: time.Minute,
		redisHost:             "localhost",
		redisPort:             6379,
		redisDB:               0,
		redisPoolSize:         20,
		redisMaxRetries:       3,
		redisDialTimeout:      5 * time.Second,
		crdtBindAddr:          ":8081",
		crdtGossipInterval:    time.Second,
		crdtSnapshotInterval:  30 * time.Second,
	}
}

func (o *storageOptions) addFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&o.backend, "storage", chronostorage.BackendMemory, "storage backend (memory, redis, crdt)")
	cmd.Flags().DurationVar(&o.memoryCleanupInterval, "storage-memory-cleanup-interval", time.Minute, "cleanup interval for memory storage backend")
	cmd.Flags().StringVar(&o.redisHost, "redis-host", "localhost", "redis host (or host:port)")
	cmd.Flags().IntVar(&o.redisPort, "redis-port", 6379, "redis port")
	cmd.Flags().StringVar(&o.redisPassword, "redis-password", "", "redis password")
	cmd.Flags().IntVar(&o.redisDB, "redis-db", 0, "redis database index")
	cmd.Flags().BoolVar(&o.redisCluster, "redis-cluster", false, "enable redis cluster mode")
	cmd.Flags().StringSliceVar(&o.redisClusterNodes, "redis-cluster-nodes", nil, "redis cluster nodes host:port list")
	cmd.Flags().IntVar(&o.redisPoolSize, "redis-pool-size", 20, "redis connection pool size")
	cmd.Flags().IntVar(&o.redisMaxRetries, "redis-max-retries", 3, "redis max retries")
	cmd.Flags().DurationVar(&o.redisDialTimeout, "redis-dial-timeout", 5*time.Second, "redis dial timeout")
	cmd.Flags().StringVar(&o.crdtNodeID, "crdt-node-id", "", "CRDT node id (required for crdt backend)")
	cmd.Flags().StringVar(&o.crdtBindAddr, "crdt-bind-addr", ":8081", "CRDT bind address")
	cmd.Flags().StringSliceVar(&o.crdtPeers, "crdt-peers", nil, "CRDT peer addresses")
	cmd.Flags().DurationVar(&o.crdtGossipInterval, "crdt-gossip-interval", time.Second, "CRDT gossip interval")
	cmd.Flags().StringVar(&o.crdtPersistDir, "crdt-persist-dir", "", "CRDT persistence directory for snapshot/WAL state")
	cmd.Flags().DurationVar(&o.crdtSnapshotInterval, "crdt-snapshot-interval", 30*time.Second, "CRDT snapshot interval when persistence is enabled")
}

func (o *storageOptions) applyConfigIfUnset(cmd *cobra.Command, cfg *config.StorageConfig) {
	if cfg == nil {
		return
	}

	if !cmd.Flags().Changed("storage") {
		o.backend = cfg.Backend
	}
	if !cmd.Flags().Changed("storage-memory-cleanup-interval") {
		o.memoryCleanupInterval = cfg.Memory.CleanupInterval
	}
	if !cmd.Flags().Changed("redis-host") {
		o.redisHost = cfg.Redis.Host
	}
	if !cmd.Flags().Changed("redis-port") {
		o.redisPort = cfg.Redis.Port
	}
	if !cmd.Flags().Changed("redis-password") {
		o.redisPassword = cfg.Redis.Password
	}
	if !cmd.Flags().Changed("redis-db") {
		o.redisDB = cfg.Redis.DB
	}
	if !cmd.Flags().Changed("redis-cluster") {
		o.redisCluster = cfg.Redis.Cluster
	}
	if !cmd.Flags().Changed("redis-cluster-nodes") {
		o.redisClusterNodes = cfg.Redis.ClusterNodes
	}
	if !cmd.Flags().Changed("redis-pool-size") {
		o.redisPoolSize = cfg.Redis.PoolSize
	}
	if !cmd.Flags().Changed("redis-max-retries") {
		o.redisMaxRetries = cfg.Redis.MaxRetries
	}
	if !cmd.Flags().Changed("redis-dial-timeout") {
		o.redisDialTimeout = cfg.Redis.DialTimeout
	}
	if !cmd.Flags().Changed("crdt-node-id") {
		o.crdtNodeID = cfg.CRDT.NodeID
	}
	if !cmd.Flags().Changed("crdt-bind-addr") {
		o.crdtBindAddr = cfg.CRDT.BindAddr
	}
	if !cmd.Flags().Changed("crdt-peers") {
		o.crdtPeers = cfg.CRDT.Peers
	}
	if !cmd.Flags().Changed("crdt-gossip-interval") {
		o.crdtGossipInterval = cfg.CRDT.GossipInterval
	}
	if !cmd.Flags().Changed("crdt-persist-dir") {
		o.crdtPersistDir = cfg.CRDT.PersistDir
	}
	if !cmd.Flags().Changed("crdt-snapshot-interval") {
		o.crdtSnapshotInterval = cfg.CRDT.SnapshotInterval
	}
}

func (o *storageOptions) normalize() error {
	if o.redisCluster {
		return nil
	}

	host, port, err := normalizeRedisHostPort(o.redisHost, o.redisPort)
	if err != nil {
		return err
	}
	o.redisHost = host
	o.redisPort = port
	return nil
}

func (o *storageOptions) toConfig(burst int) config.StorageConfig {
	return config.StorageConfig{
		Backend: o.backend,
		Memory: config.StorageMemoryConfig{
			CleanupInterval: o.memoryCleanupInterval,
			Burst:           burst,
		},
		Redis: config.StorageRedisConfig{
			Host:         o.redisHost,
			Port:         o.redisPort,
			Password:     o.redisPassword,
			DB:           o.redisDB,
			Cluster:      o.redisCluster,
			ClusterNodes: append([]string(nil), o.redisClusterNodes...),
			PoolSize:     o.redisPoolSize,
			MaxRetries:   o.redisMaxRetries,
			DialTimeout:  o.redisDialTimeout,
		},
		CRDT: config.StorageCRDTConfig{
			NodeID:           o.crdtNodeID,
			BindAddr:         o.crdtBindAddr,
			Peers:            append([]string(nil), o.crdtPeers...),
			GossipInterval:   o.crdtGossipInterval,
			PersistDir:       o.crdtPersistDir,
			SnapshotInterval: o.crdtSnapshotInterval,
		},
	}
}

func normalizeRedisHostPort(host string, port int) (string, int, error) {
	if strings.Contains(host, ":") {
		h, p, err := net.SplitHostPort(host)
		if err != nil {
			return "", 0, fmt.Errorf("invalid --redis-host value %q: %w", host, err)
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return "", 0, fmt.Errorf("invalid redis port in --redis-host %q: %w", host, err)
		}
		host = h
		port = n
	}

	if host == "" {
		return "", 0, fmt.Errorf("redis host cannot be empty")
	}
	if port <= 0 {
		return "", 0, fmt.Errorf("redis port must be positive, got %d", port)
	}

	return host, port, nil
}
