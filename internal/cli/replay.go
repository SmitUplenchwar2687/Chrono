package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SmitUplenchwar2687/Chrono/internal/clock"
	"github.com/SmitUplenchwar2687/Chrono/internal/limiter"
	"github.com/SmitUplenchwar2687/Chrono/internal/replay"
)

func newReplayCmd() *cobra.Command {
	var (
		file       string
		algorithm  string
		rate       int
		window     time.Duration
		burst      int
		speed      float64
		keys       []string
		endpoints  []string
		outputJSON bool
	)

	cmd := &cobra.Command{
		Use:   "replay",
		Short: "Replay recorded traffic through a rate limiter",
		Long: `Replays previously recorded traffic through a rate limiter with speed control.

Records are replayed in timestamp order. The virtual clock advances
to match the time gaps between records, so rate limiting behaves
exactly as it would in production — but at any speed you choose.

Speed: 0 = instant, 1 = real-time, 10 = 10x, 100 = 100x`,
		Example: `  chrono replay --file traffic.json
  chrono replay --file traffic.json --speed 100 --algorithm sliding_window
  chrono replay --file traffic.json --keys user1,user2 --endpoints /api
  chrono replay --file traffic.json --speed 0 --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("--file is required")
			}

			f, err := os.Open(file)
			if err != nil {
				return fmt.Errorf("opening file: %w", err)
			}
			defer f.Close()

			vc := clock.NewVirtualClock(time.Now().Truncate(time.Second))
			lim, err := createLimiter(limiter.Algorithm(algorithm), rate, window, burst, vc)
			if err != nil {
				return err
			}

			filter := &replay.Filter{
				Keys:      keys,
				Endpoints: endpoints,
			}

			r := replay.New(lim, vc, speed, filter)
			if err := r.Load(f); err != nil {
				return err
			}

			if !outputJSON {
				fmt.Printf("Replaying %s at %.0fx speed...\n\n", file, speed)
			}

			var results []replay.Result
			summary, err := r.Run(context.Background(), func(res replay.Result) {
				if outputJSON {
					results = append(results, res)
					return
				}
				status := "ALLOW"
				if !res.Decision.Allowed {
					status = "DENY "
				}
				fmt.Printf("  [%s] %s key=%s remaining=%d/%d\n",
					status,
					res.Record.Timestamp.Format("15:04:05"),
					res.Record.Key,
					res.Decision.Remaining,
					res.Decision.Limit)
			})
			if err != nil {
				return err
			}

			if outputJSON {
				out := map[string]interface{}{
					"results": results,
					"summary": summary,
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}

			fmt.Println()
			fmt.Println("--- Replay Summary ---")
			fmt.Printf("  Total records:  %d\n", summary.TotalRecords)
			fmt.Printf("  Filtered:       %d\n", summary.Filtered)
			fmt.Printf("  Replayed:       %d\n", summary.Replayed)
			fmt.Printf("  Allowed:        %d\n", summary.Allowed)
			fmt.Printf("  Denied:         %d\n", summary.Denied)
			fmt.Printf("  Virtual time:   %s\n", summary.Duration)
			fmt.Printf("  Wall time:      %s\n", summary.WallDuration.Round(time.Millisecond))

			if len(summary.PerKey) > 1 {
				fmt.Println()
				fmt.Println("  Per key:")
				for key, ks := range summary.PerKey {
					fmt.Printf("    %s: %d allowed, %d denied\n", key, ks.Allowed, ks.Denied)
				}
			}

			if summary.Denied > 0 && summary.Allowed > 0 {
				fmt.Println()
				fmt.Println(strings.Repeat("=", 50))
				denyRate := float64(summary.Denied) / float64(summary.Replayed) * 100
				fmt.Printf("Deny rate: %.1f%% (%d/%d requests denied)\n", denyRate, summary.Denied, summary.Replayed)
				fmt.Println(strings.Repeat("=", 50))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&file, "file", "", "path to recorded traffic JSON file (required)")
	cmd.Flags().StringVar(&algorithm, "algorithm", "token_bucket", "rate limiting algorithm")
	cmd.Flags().IntVar(&rate, "rate", 10, "requests allowed per window")
	cmd.Flags().DurationVar(&window, "window", time.Minute, "rate limit window duration")
	cmd.Flags().IntVar(&burst, "burst", 0, "max burst size (token_bucket only)")
	cmd.Flags().Float64Var(&speed, "speed", 0, "replay speed (0=instant, 1=real-time, 10=10x)")
	cmd.Flags().StringSliceVar(&keys, "keys", nil, "filter by keys (comma-separated)")
	cmd.Flags().StringSliceVar(&endpoints, "endpoints", nil, "filter by endpoints (comma-separated)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "output results as JSON")

	return cmd
}
