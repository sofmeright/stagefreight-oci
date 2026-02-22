package cmd

import (
	"github.com/spf13/cobra"
)

var dockerCmd = &cobra.Command{
	Use:   "docker",
	Short: "Docker image commands",
	Long:  "Build, push, and manage container images.",
}

func init() {
	rootCmd.AddCommand(dockerCmd)
}
