package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/SmitUplenchwar2687/Chrono/internal/config"
	pkggenerate "github.com/SmitUplenchwar2687/Chrono/pkg/generate"
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

			records, err := pkggenerate.GenerateTraffic(pkggenerate.Options{
				Count:    count,
				Keys:     keys,
				Duration: duration,
				Pattern:  pattern,
			})
			if err != nil {
				return err
			}

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
