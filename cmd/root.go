package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "openpraxis",
	Short: "P2P shared memory for coding agents",
	Long: `OpenPraxis — a peer-to-peer distributed memory layer
for coding agents. Stores, searches, and replicates memories
across agents (Claude Code, Cursor, Copilot, Windsurf) and
across machines on the same network.

Run 'openpraxis serve' to start the daemon and open the dashboard.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default ~/.openpraxis/config.yaml)")

	// Default command is serve — run daemon if no subcommand given
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return serveCmd.RunE(cmd, args)
	}
}

func exitErr(msg string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", msg, err)
	os.Exit(1)
}
