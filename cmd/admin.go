package cmd

import "github.com/spf13/cobra"

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "One-shot administrative commands",
	Long:  "Administrative utilities for maintaining an OpenPraxis data directory.",
}

func init() {
	rootCmd.AddCommand(adminCmd)
}
