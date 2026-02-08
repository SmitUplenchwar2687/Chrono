package main

import (
	"os"

	"github.com/SmitUplenchwar2687/Chrono/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
