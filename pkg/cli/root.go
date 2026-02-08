package cli

import (
	internalcli "github.com/SmitUplenchwar2687/Chrono/internal/cli"
	"github.com/spf13/cobra"
)

// NewRootCmd creates the public Chrono root command for embedding.
func NewRootCmd() *cobra.Command {
	return internalcli.NewRootCmd()
}
