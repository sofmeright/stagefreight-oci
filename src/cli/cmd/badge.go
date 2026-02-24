package cmd

import (
	"github.com/spf13/cobra"
)

var badgeCmd = &cobra.Command{
	Use:   "badge",
	Short: "Badge generation commands",
	Long:  "Generate SVG badges from config or ad-hoc flags.",
}

func init() {
	rootCmd.AddCommand(badgeCmd)
}
