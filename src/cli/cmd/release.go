package cmd

import (
	"github.com/spf13/cobra"
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Release management commands",
	Long:  "Create releases, generate notes, update badges, and sync across forges.",
}

func init() {
	rootCmd.AddCommand(releaseCmd)
}
