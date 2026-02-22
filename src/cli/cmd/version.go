package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"gitlab.prplanit.com/precisionplanit/stagefreight-oci/src/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(version.String())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
