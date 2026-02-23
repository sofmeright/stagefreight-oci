package cmd

import (
	"github.com/spf13/cobra"
)

var componentCmd = &cobra.Command{
	Use:   "component",
	Short: "GitLab CI component management",
	Long:  "Parse component specs, generate documentation, and manage component releases.",
}

func init() {
	rootCmd.AddCommand(componentCmd)
}
