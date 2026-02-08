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
)

func newTestCmd() *cobra.Command {
	var (
		algorithm    string
		rate         int
		window       time.Duration
		burst        int
		requests     int
		keys         []string
		fastForward  time.Duration
		outputJSON   bool
	)

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run rate limit test scenarios with time travel",
		Long: `Runs rate limit checks against a virtual clock, allowing you to
fast-forward time without waiting. This lets you verify rate limiter
behavior over hours or days in seconds.

The test sends a batch of requests, optionally fast-forwards time,
then sends another batch to show how limits reset.`,
		Example: `  chrono test --requests 20 --rate 10 --window 1m
  chrono test --algorithm sliding_window --rate 5 --window 30s --fast-forward 1m
  chrono test --keys user1,user2 --requests 15 --rate 10 --window 1m --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(keys) == 0 {
				keys = []string{"test-user"}
			}

			vc := clock.NewVirtualClock(time.Now().Truncate(time.Second))
			lim, err := createLimiter(limiter.Algorithm(algorithm), rate, window, burst, vc)
			if err != nil {
				return err
			}

			result := runTest(vc, lim, keys, requests, fastForward)

			if outputJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}

			printTestResult(&result)
			return nil
		},
	}

	cmd.Flags().StringVar(&algorithm, "algorithm", "token_bucket", "rate limiting algorithm")
	cmd.Flags().IntVar(&rate, "rate", 10, "requests allowed per window")
	cmd.Flags().DurationVar(&window, "window", time.Minute, "rate limit window duration")
	cmd.Flags().IntVar(&burst, "burst", 0, "max burst size (token_bucket only)")
	cmd.Flags().IntVar(&requests, "requests", 15, "number of requests to send per batch")
	cmd.Flags().StringSliceVar(&keys, "keys", nil, "comma-separated rate limit keys to test")
	cmd.Flags().DurationVar(&fastForward, "fast-forward", 0, "time to fast-forward between batches")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "output results as JSON")

	return cmd
}

// TestResult captures the full output of a test run.
type TestResult struct {
	Algorithm   string            `json:"algorithm"`
	Rate        int               `json:"rate"`
	Window      string            `json:"window"`
	FastForward string            `json:"fast_forward,omitempty"`
	Batches     []BatchResult     `json:"batches"`
	Summary     map[string]Summary `json:"summary"`
}

// BatchResult captures results for one batch of requests.
type BatchResult struct {
	Label     string           `json:"label"`
	Time      string           `json:"time"`
	Decisions []DecisionRecord `json:"decisions"`
}

// DecisionRecord is a single rate limit check result.
type DecisionRecord struct {
	Key       string           `json:"key"`
	Decision  limiter.Decision `json:"decision"`
}

// Summary aggregates stats per key.
type Summary struct {
	TotalRequests int `json:"total_requests"`
	Allowed       int `json:"allowed"`
	Denied        int `json:"denied"`
}

func runTest(vc *clock.VirtualClock, lim limiter.Limiter, keys []string, requests int, fastForward time.Duration) TestResult {
	ctx := context.Background()

	result := TestResult{
		Summary: make(map[string]Summary),
	}

	// Batch 1: initial requests.
	batch1 := BatchResult{
		Label: "Initial requests",
		Time:  vc.Now().Format(time.RFC3339),
	}
	for i := 0; i < requests; i++ {
		for _, key := range keys {
			d := lim.Allow(ctx, key)
			batch1.Decisions = append(batch1.Decisions, DecisionRecord{Key: key, Decision: d})
			s := result.Summary[key]
			s.TotalRequests++
			if d.Allowed {
				s.Allowed++
			} else {
				s.Denied++
			}
			result.Summary[key] = s
		}
	}
	result.Batches = append(result.Batches, batch1)

	// Fast-forward if requested.
	if fastForward > 0 {
		vc.Advance(fastForward)
		result.FastForward = fastForward.String()

		// Batch 2: after time travel.
		batch2 := BatchResult{
			Label: fmt.Sprintf("After fast-forward %s", fastForward),
			Time:  vc.Now().Format(time.RFC3339),
		}
		for i := 0; i < requests; i++ {
			for _, key := range keys {
				d := lim.Allow(ctx, key)
				batch2.Decisions = append(batch2.Decisions, DecisionRecord{Key: key, Decision: d})
				s := result.Summary[key]
				s.TotalRequests++
				if d.Allowed {
					s.Allowed++
				} else {
					s.Denied++
				}
				result.Summary[key] = s
			}
		}
		result.Batches = append(result.Batches, batch2)
	}

	return result
}

func printTestResult(r *TestResult) {
	fmt.Println("=== Chrono Rate Limit Test ===")
	fmt.Println()

	for _, batch := range r.Batches {
		fmt.Printf("--- %s (at %s) ---\n", batch.Label, batch.Time)
		for i, dr := range batch.Decisions {
			status := "ALLOW"
			if !dr.Decision.Allowed {
				status = "DENY "
			}
			fmt.Printf("  #%03d [%s] key=%s remaining=%d/%d\n",
				i+1, status, dr.Key, dr.Decision.Remaining, dr.Decision.Limit)
		}
		fmt.Println()
	}

	fmt.Println("--- Summary ---")
	for key, s := range r.Summary {
		fmt.Printf("  %s: %d total, %d allowed, %d denied\n",
			key, s.TotalRequests, s.Allowed, s.Denied)
	}

	if r.FastForward != "" {
		fmt.Printf("\nTime travel: fast-forwarded %s\n", r.FastForward)
	}

	// Check if any denials happened then recoveries — the key insight.
	hasDenials := false
	hasRecovery := false
	for _, batch := range r.Batches {
		for _, dr := range batch.Decisions {
			if !dr.Decision.Allowed {
				hasDenials = true
			}
		}
	}
	if len(r.Batches) > 1 {
		for _, dr := range r.Batches[1].Decisions {
			if dr.Decision.Allowed {
				hasRecovery = true
				break
			}
		}
	}
	if hasDenials && hasRecovery {
		fmt.Println()
		fmt.Println(strings.Repeat("=", 50))
		fmt.Println("Time travel worked! Requests were denied, then")
		fmt.Println("allowed again after fast-forwarding the clock.")
		fmt.Println(strings.Repeat("=", 50))
	}
}
