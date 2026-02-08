package cli

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/SmitUplenchwar2687/Chrono/internal/config"
	"github.com/SmitUplenchwar2687/Chrono/internal/recorder"
)

func newGenerateCmd() *cobra.Command {
	var (
		output   string
		count    int
		keys     int
		duration time.Duration
		pattern  string
	)

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate sample traffic files and config",
		Long: `Generates sample data for testing and experimentation.

Use "generate traffic" to create a sample traffic JSON file.
Use "generate config" to create an example config JSON file.`,
	}

	trafficCmd := &cobra.Command{
		Use:   "traffic",
		Short: "Generate a sample traffic JSON file",
		Long: `Creates a realistic traffic file with configurable parameters.

Patterns:
  steady    Evenly distributed requests
  burst     Concentrated bursts with quiet periods
  ramp      Gradually increasing request rate`,
		Example: `  chrono generate traffic --output traffic.json --count 100 --keys 5
  chrono generate traffic --output burst.json --count 200 --pattern burst --duration 10m`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if output == "" {
				output = "traffic.json"
			}

			records := generateTraffic(count, keys, duration, pattern)

			f, err := os.Create(output)
			if err != nil {
				return fmt.Errorf("creating file: %w", err)
			}
			defer f.Close()

			enc := json.NewEncoder(f)
			enc.SetIndent("", "  ")
			if err := enc.Encode(records); err != nil {
				return fmt.Errorf("writing records: %w", err)
			}

			fmt.Printf("Generated %d traffic records to %s\n", len(records), output)
			fmt.Printf("  Keys:     %d\n", keys)
			fmt.Printf("  Duration: %s\n", duration)
			fmt.Printf("  Pattern:  %s\n", pattern)
			return nil
		},
	}

	trafficCmd.Flags().StringVar(&output, "output", "traffic.json", "output file path")
	trafficCmd.Flags().IntVar(&count, "count", 100, "number of records to generate")
	trafficCmd.Flags().IntVar(&keys, "keys", 3, "number of distinct user keys")
	trafficCmd.Flags().DurationVar(&duration, "duration", 5*time.Minute, "time span for generated traffic")
	trafficCmd.Flags().StringVar(&pattern, "pattern", "steady", "traffic pattern (steady, burst, ramp)")

	configCmd := &cobra.Command{
		Use:     "config",
		Short:   "Generate an example config JSON file",
		Example: `  chrono generate config --output chrono.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if output == "" {
				output = "chrono.json"
			}
			if err := config.WriteExample(output); err != nil {
				return err
			}
			fmt.Printf("Generated example config at %s\n", output)
			return nil
		},
	}

	configCmd.Flags().StringVar(&output, "output", "chrono.json", "output file path")

	cmd.AddCommand(trafficCmd, configCmd)
	return cmd
}

var endpoints = []string{
	"GET /api/users",
	"GET /api/data",
	"POST /api/events",
	"GET /api/search",
	"PUT /api/settings",
}

func generateTraffic(count, numKeys int, duration time.Duration, pattern string) []recorder.TrafficRecord {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	start := time.Now().Truncate(time.Second)

	userKeys := make([]string, numKeys)
	for i := range userKeys {
		userKeys[i] = fmt.Sprintf("user-%d", i+1)
	}

	records := make([]recorder.TrafficRecord, 0, count)

	switch pattern {
	case "burst":
		records = generateBurst(rng, start, count, userKeys, duration)
	case "ramp":
		records = generateRamp(rng, start, count, userKeys, duration)
	default: // "steady"
		records = generateSteady(rng, start, count, userKeys, duration)
	}

	return records
}

func generateSteady(rng *rand.Rand, start time.Time, count int, keys []string, dur time.Duration) []recorder.TrafficRecord {
	interval := dur / time.Duration(count)
	records := make([]recorder.TrafficRecord, count)
	for i := range records {
		records[i] = recorder.TrafficRecord{
			Timestamp: start.Add(time.Duration(i) * interval),
			Key:       keys[rng.Intn(len(keys))],
			Endpoint:  endpoints[rng.Intn(len(endpoints))],
		}
	}
	return records
}

func generateBurst(rng *rand.Rand, start time.Time, count int, keys []string, dur time.Duration) []recorder.TrafficRecord {
	records := make([]recorder.TrafficRecord, 0, count)
	numBursts := 4
	burstSize := count / numBursts
	burstGap := dur / time.Duration(numBursts)

	for b := 0; b < numBursts; b++ {
		burstStart := start.Add(time.Duration(b) * burstGap)
		for i := 0; i < burstSize; i++ {
			// Requests within a burst are very close together.
			offset := time.Duration(rng.Intn(1000)) * time.Millisecond
			records = append(records, recorder.TrafficRecord{
				Timestamp: burstStart.Add(offset),
				Key:       keys[rng.Intn(len(keys))],
				Endpoint:  endpoints[rng.Intn(len(endpoints))],
			})
		}
	}

	// Fill remaining.
	for len(records) < count {
		records = append(records, recorder.TrafficRecord{
			Timestamp: start.Add(time.Duration(rng.Int63n(int64(dur)))),
			Key:       keys[rng.Intn(len(keys))],
			Endpoint:  endpoints[rng.Intn(len(endpoints))],
		})
	}

	return records
}

func generateRamp(rng *rand.Rand, start time.Time, count int, keys []string, dur time.Duration) []recorder.TrafficRecord {
	records := make([]recorder.TrafficRecord, 0, count)
	// Use quadratic distribution: more requests towards the end.
	for i := 0; i < count; i++ {
		// t is proportional to sqrt(i/count), concentrating records towards the end.
		frac := float64(i) / float64(count)
		t := start.Add(time.Duration(frac * frac * float64(dur)))
		records = append(records, recorder.TrafficRecord{
			Timestamp: t,
			Key:       keys[rng.Intn(len(keys))],
			Endpoint:  endpoints[rng.Intn(len(endpoints))],
		})
	}
	return records
}
