package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
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
		addr       string
		algorithm  string
		rate       int
		window     time.Duration
		burst      int
		recordFile string
		configFile string
	)
	storageOpts := defaultStorageOptions()

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
				storageOpts.applyConfigIfUnset(cmd, &cfg.Storage)
			}

			if err := storageOpts.normalize(); err != nil {
				return err
			}
			storageCfg := storageOpts.toConfig(burst)

			clk := clock.NewRealClock()
			lim, err := createServerLimiter(limiter.Algorithm(algorithm), rate, window, burst, clk, &storageCfg)
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
	storageOpts.addFlags(cmd)
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
	storageCfg *config.StorageConfig,
) (limiter.Limiter, error) {
	if storageCfg == nil {
		return nil, fmt.Errorf("storage config is required")
	}

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
			Clock:        clk,
		}
	case chronostorage.BackendCRDT:
		if algo != limiter.AlgorithmSlidingWindow {
			return nil, fmt.Errorf("algorithm %q is unsupported with crdt backend; use %q", algo, limiter.AlgorithmSlidingWindow)
		}
		cfg.CRDT = &chronostorage.CRDTConfig{
			NodeID:           storageCfg.CRDT.NodeID,
			BindAddr:         storageCfg.CRDT.BindAddr,
			Peers:            append([]string(nil), storageCfg.CRDT.Peers...),
			GossipInterval:   storageCfg.CRDT.GossipInterval,
			PersistDir:       storageCfg.CRDT.PersistDir,
			SnapshotInterval: storageCfg.CRDT.SnapshotInterval,
			Clock:            clk,
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
