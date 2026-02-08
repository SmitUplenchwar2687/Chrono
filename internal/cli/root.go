package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCmd creates the root chrono command.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "chrono",
		Short: "Time-travel testing for rate limits",
		Long: `Chrono lets you test rate limiting without waiting for real time to pass.
Fast-forward time, replay traffic, and visualize rate limit decisions.`,
		SilenceUsage: true,
	}

	root.AddCommand(
		newServerCmd(),
		newTestCmd(),
		newReplayCmd(),
		newDashboardCmd(),
		newGenerateCmd(),
	)

	return root
}
