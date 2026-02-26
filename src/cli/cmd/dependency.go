package cmd

import (
	"github.com/spf13/cobra"
)

var dependencyCmd = &cobra.Command{
	Use:     "dependency",
	Aliases: []string{"deps"},
	Short:   "Dependency management commands",
	Long:    "Resolve, update, and audit project dependencies.",
}

func init() {
	rootCmd.AddCommand(dependencyCmd)
}
