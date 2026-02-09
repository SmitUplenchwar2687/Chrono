package cli

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
	"github.com/SmitUplenchwar2687/Chrono/internal/config"
	"github.com/SmitUplenchwar2687/Chrono/internal/limiter"
	"github.com/SmitUplenchwar2687/Chrono/internal/recorder"
	"github.com/SmitUplenchwar2687/Chrono/internal/server"
	chronostorage "github.com/SmitUplenchwar2687/Chrono/pkg/storage"
)

func newServerCmd() *cobra.Command {
	var (
		addr                  string
		algorithm             string
		rate                  int
		window                time.Duration
		burst                 int
		recordFile            string
		configFile            string
		storageBackend        string
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
	)

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start the Chrono HTTP server with rate limiting",
		Long: `Starts an HTTP server that applies rate limiting to incoming requests.

Endpoints:
  GET /                  Server info and current time
  GET /health            Health check
  GET /api/check         Check rate limit using client IP
  GET /api/check/:key    Check rate limit for a specific key
  GET /dashboard/        Live visual dashboard
  WS  /ws                WebSocket for real-time events`,
		Example: `  chrono server
  chrono server --addr :9090 --algorithm sliding_window --rate 100 --window 1m
  chrono server --algorithm token_bucket --rate 10 --window 1m --burst 20
  chrono server --storage redis --algorithm sliding_window --redis-host localhost:6379
  chrono server --storage crdt --algorithm sliding_window --crdt-node-id node-1 --crdt-bind-addr :8081
  chrono server --record traffic.json
  chrono server --config chrono.json
  chrono server --config chrono.json --rate 20  # flag overrides config`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load config file if specified; flags override config values.
			if configFile != "" {
				cfg, err := config.LoadFile(configFile)
				if err != nil {
					return err
				}
				if err := cfg.Validate(); err != nil {
					return fmt.Errorf("invalid config: %w", err)
				}
				// Only apply config values for flags that weren't explicitly set.
				if !cmd.Flags().Changed("addr") {
					addr = cfg.Server.Addr
				}
				if !cmd.Flags().Changed("algorithm") {
					algorithm = string(cfg.Limiter.Algorithm)
				}
				if !cmd.Flags().Changed("rate") {
					rate = cfg.Limiter.Rate
				}
				if !cmd.Flags().Changed("window") {
					window = cfg.Limiter.Window
				}
				if !cmd.Flags().Changed("burst") {
					burst = cfg.Limiter.Burst
				}
				if !cmd.Flags().Changed("storage") {
					storageBackend = cfg.Storage.Backend
				}
				if !cmd.Flags().Changed("storage-memory-cleanup-interval") {
					memoryCleanupInterval = cfg.Storage.Memory.CleanupInterval
				}
				if !cmd.Flags().Changed("redis-host") {
					redisHost = cfg.Storage.Redis.Host
				}
				if !cmd.Flags().Changed("redis-port") {
					redisPort = cfg.Storage.Redis.Port
				}
				if !cmd.Flags().Changed("redis-password") {
					redisPassword = cfg.Storage.Redis.Password
				}
				if !cmd.Flags().Changed("redis-db") {
					redisDB = cfg.Storage.Redis.DB
				}
				if !cmd.Flags().Changed("redis-cluster") {
					redisCluster = cfg.Storage.Redis.Cluster
				}
				if !cmd.Flags().Changed("redis-cluster-nodes") {
					redisClusterNodes = cfg.Storage.Redis.ClusterNodes
				}
				if !cmd.Flags().Changed("redis-pool-size") {
					redisPoolSize = cfg.Storage.Redis.PoolSize
				}
				if !cmd.Flags().Changed("redis-max-retries") {
					redisMaxRetries = cfg.Storage.Redis.MaxRetries
				}
				if !cmd.Flags().Changed("redis-dial-timeout") {
					redisDialTimeout = cfg.Storage.Redis.DialTimeout
				}
				if !cmd.Flags().Changed("crdt-node-id") {
					crdtNodeID = cfg.Storage.CRDT.NodeID
				}
				if !cmd.Flags().Changed("crdt-bind-addr") {
					crdtBindAddr = cfg.Storage.CRDT.BindAddr
				}
				if !cmd.Flags().Changed("crdt-peers") {
					crdtPeers = cfg.Storage.CRDT.Peers
				}
				if !cmd.Flags().Changed("crdt-gossip-interval") {
					crdtGossipInterval = cfg.Storage.CRDT.GossipInterval
				}
			}

			var err error
			if !redisCluster {
				redisHost, redisPort, err = normalizeRedisHostPort(redisHost, redisPort)
				if err != nil {
					return err
				}
			}

			storageCfg := config.StorageConfig{
				Backend: storageBackend,
				Memory: config.StorageMemoryConfig{
					CleanupInterval: memoryCleanupInterval,
					Burst:           burst,
				},
				Redis: config.StorageRedisConfig{
					Host:         redisHost,
					Port:         redisPort,
					Password:     redisPassword,
					DB:           redisDB,
					Cluster:      redisCluster,
					ClusterNodes: append([]string(nil), redisClusterNodes...),
					PoolSize:     redisPoolSize,
					MaxRetries:   redisMaxRetries,
					DialTimeout:  redisDialTimeout,
				},
				CRDT: config.StorageCRDTConfig{
					NodeID:         crdtNodeID,
					BindAddr:       crdtBindAddr,
					Peers:          append([]string(nil), crdtPeers...),
					GossipInterval: crdtGossipInterval,
				},
			}

			clk := clock.NewRealClock()
			lim, err := createServerLimiter(limiter.Algorithm(algorithm), rate, window, burst, clk, storageCfg)
			if err != nil {
				return err
			}
			if c, ok := lim.(interface{ Close() error }); ok {
				defer func() {
					if err := c.Close(); err != nil {
						log.Printf("storage close error: %v", err)
					}
				}()
			}

			opts := server.Options{
				Hub: server.NewHub(),
			}

			if recordFile != "" {
				opts.Recorder = recorder.New(nil)
			}

			srv := server.New(addr, lim, clk, opts)

			log.Printf("Dashboard: http://localhost%s/dashboard/", addr)
			log.Printf("API:       http://localhost%s/api/check/{key}", addr)

			// Graceful shutdown on SIGINT/SIGTERM.
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			errCh := make(chan error, 1)
			go func() {
				errCh <- srv.Start()
			}()

			select {
			case err := <-errCh:
				return err
			case <-ctx.Done():
				log.Println("shutting down...")
				// Export recordings if enabled.
				if recordFile != "" && opts.Recorder != nil {
					log.Printf("exporting %d records to %s", opts.Recorder.Len(), recordFile)
					if err := opts.Recorder.ExportFile(recordFile); err != nil {
						log.Printf("error exporting records: %v", err)
					}
				}
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				return srv.Shutdown(shutdownCtx)
			}
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8080", "address to listen on")
	cmd.Flags().StringVar(&algorithm, "algorithm", "token_bucket", "rate limiting algorithm (token_bucket, sliding_window, fixed_window)")
	cmd.Flags().IntVar(&rate, "rate", 10, "requests allowed per window")
	cmd.Flags().DurationVar(&window, "window", time.Minute, "rate limit window duration")
	cmd.Flags().IntVar(&burst, "burst", 0, "max burst size (token_bucket only, 0 = same as rate)")
	cmd.Flags().StringVar(&storageBackend, "storage", chronostorage.BackendMemory, "storage backend (memory, redis, crdt)")
	cmd.Flags().DurationVar(&memoryCleanupInterval, "storage-memory-cleanup-interval", time.Minute, "cleanup interval for memory storage backend")
	cmd.Flags().StringVar(&redisHost, "redis-host", "localhost", "redis host (or host:port)")
	cmd.Flags().IntVar(&redisPort, "redis-port", 6379, "redis port")
	cmd.Flags().StringVar(&redisPassword, "redis-password", "", "redis password")
	cmd.Flags().IntVar(&redisDB, "redis-db", 0, "redis database index")
	cmd.Flags().BoolVar(&redisCluster, "redis-cluster", false, "enable redis cluster mode")
	cmd.Flags().StringSliceVar(&redisClusterNodes, "redis-cluster-nodes", nil, "redis cluster nodes host:port list")
	cmd.Flags().IntVar(&redisPoolSize, "redis-pool-size", 20, "redis connection pool size")
	cmd.Flags().IntVar(&redisMaxRetries, "redis-max-retries", 3, "redis max retries")
	cmd.Flags().DurationVar(&redisDialTimeout, "redis-dial-timeout", 5*time.Second, "redis dial timeout")
	cmd.Flags().StringVar(&crdtNodeID, "crdt-node-id", "", "CRDT node id (required for crdt backend)")
	cmd.Flags().StringVar(&crdtBindAddr, "crdt-bind-addr", ":8081", "CRDT bind address")
	cmd.Flags().StringSliceVar(&crdtPeers, "crdt-peers", nil, "CRDT peer addresses")
	cmd.Flags().DurationVar(&crdtGossipInterval, "crdt-gossip-interval", time.Second, "CRDT gossip interval")
	cmd.Flags().StringVar(&recordFile, "record", "", "record traffic to JSON file (exported on shutdown)")
	cmd.Flags().StringVar(&configFile, "config", "", "path to JSON config file (flags override config values)")

	return cmd
}

func createServerLimiter(
	algo limiter.Algorithm,
	rate int,
	window time.Duration,
	burst int,
	clk clock.Clock,
	storageCfg config.StorageConfig,
) (limiter.Limiter, error) {
	backend := storageCfg.Backend
	if backend == "" {
		backend = chronostorage.BackendMemory
	}

	cfg := chronostorage.Config{Backend: backend}
	switch backend {
	case chronostorage.BackendMemory:
		cfg.Memory = &chronostorage.MemoryConfig{
			CleanupInterval: storageCfg.Memory.CleanupInterval,
			Algorithm:       string(algo),
			Burst:           burst,
			Clock:           clk,
		}
	case chronostorage.BackendRedis:
		if algo != limiter.AlgorithmSlidingWindow {
			return nil, fmt.Errorf("algorithm %q is unsupported with redis backend; use %q", algo, limiter.AlgorithmSlidingWindow)
		}
		cfg.Redis = &chronostorage.RedisConfig{
			Host:         storageCfg.Redis.Host,
			Port:         storageCfg.Redis.Port,
			Password:     storageCfg.Redis.Password,
			DB:           storageCfg.Redis.DB,
			Cluster:      storageCfg.Redis.Cluster,
			ClusterNodes: append([]string(nil), storageCfg.Redis.ClusterNodes...),
			PoolSize:     storageCfg.Redis.PoolSize,
			MaxRetries:   storageCfg.Redis.MaxRetries,
			DialTimeout:  storageCfg.Redis.DialTimeout,
		}
	case chronostorage.BackendCRDT:
		if algo != limiter.AlgorithmSlidingWindow {
			return nil, fmt.Errorf("algorithm %q is unsupported with crdt backend; use %q", algo, limiter.AlgorithmSlidingWindow)
		}
		cfg.CRDT = &chronostorage.CRDTConfig{
			NodeID:         storageCfg.CRDT.NodeID,
			BindAddr:       storageCfg.CRDT.BindAddr,
			Peers:          append([]string(nil), storageCfg.CRDT.Peers...),
			GossipInterval: storageCfg.CRDT.GossipInterval,
		}
	default:
		return nil, fmt.Errorf("unknown storage backend %q", backend)
	}

	store, err := chronostorage.NewStorage(cfg)
	if err != nil {
		return nil, err
	}

	lim, err := limiter.NewStorageLimiter(store, rate, window, clk)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	return lim, nil
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

func createLimiter(algo limiter.Algorithm, rate int, window time.Duration, burst int, clk clock.Clock) (limiter.Limiter, error) {
	switch algo {
	case limiter.AlgorithmTokenBucket:
		return limiter.NewTokenBucket(rate, window, burst, clk), nil
	case limiter.AlgorithmSlidingWindow:
		return limiter.NewSlidingWindow(rate, window, clk), nil
	case limiter.AlgorithmFixedWindow:
		return limiter.NewFixedWindow(rate, window, clk), nil
	default:
		return nil, fmt.Errorf("unknown algorithm %q", algo)
	}
}
