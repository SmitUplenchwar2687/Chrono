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
	"github.com/SmitUplenchwar2687/Chrono/internal/limiter"
	"github.com/SmitUplenchwar2687/Chrono/internal/recorder"
	"github.com/SmitUplenchwar2687/Chrono/internal/server"
)

func newServerCmd() *cobra.Command {
	var (
		addr      string
		algorithm string
		rate      int
		window    time.Duration
		burst     int
		recordFile string
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
  chrono server --record traffic.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			clk := clock.NewRealClock()
			lim, err := createLimiter(limiter.Algorithm(algorithm), rate, window, burst, clk)
			if err != nil {
				return err
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
	cmd.Flags().StringVar(&recordFile, "record", "", "record traffic to JSON file (exported on shutdown)")

	return cmd
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
