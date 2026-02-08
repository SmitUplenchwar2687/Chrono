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

func newDashboardCmd() *cobra.Command {
	var (
		addr      string
		algorithm string
		rate      int
		window    time.Duration
		burst     int
	)

	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Start the server with a live visual dashboard",
		Long: `Starts the Chrono HTTP server with WebSocket-powered live dashboard.

The dashboard shows rate limit decisions in real time as they happen.
Send requests to /api/check/{key} and watch them appear on the dashboard.

Open your browser to http://localhost:<port>/dashboard/ to view.`,
		Example: `  chrono dashboard
  chrono dashboard --addr :9090 --rate 5 --window 30s
  chrono dashboard --algorithm sliding_window --rate 20 --window 1m`,
		RunE: func(cmd *cobra.Command, args []string) error {
			clk := clock.NewRealClock()
			lim, err := createLimiter(limiter.Algorithm(algorithm), rate, window, burst, clk)
			if err != nil {
				return err
			}

			hub := server.NewHub()
			rec := recorder.New(nil)

			srv := server.New(addr, lim, clk, server.Options{
				Hub:      hub,
				Recorder: rec,
			})

			fmt.Printf("\n  Chrono Dashboard\n")
			fmt.Printf("  ────────────────────────────────────\n")
			fmt.Printf("  Dashboard:  http://localhost%s/dashboard/\n", addr)
			fmt.Printf("  API:        http://localhost%s/api/check/{key}\n", addr)
			fmt.Printf("  WebSocket:  ws://localhost%s/ws\n", addr)
			fmt.Printf("  Algorithm:  %s (%d req/%s)\n", algorithm, rate, window)
			fmt.Printf("  ────────────────────────────────────\n\n")

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
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				return srv.Shutdown(shutdownCtx)
			}
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8080", "address to listen on")
	cmd.Flags().StringVar(&algorithm, "algorithm", "token_bucket", "rate limiting algorithm")
	cmd.Flags().IntVar(&rate, "rate", 10, "requests allowed per window")
	cmd.Flags().DurationVar(&window, "window", time.Minute, "rate limit window duration")
	cmd.Flags().IntVar(&burst, "burst", 0, "max burst size (token_bucket only)")

	return cmd
}
