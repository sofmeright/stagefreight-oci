package cmd

import (
	"github.com/spf13/cobra"
)

var narratorCmd = &cobra.Command{
	Use:   "narrator",
	Short: "Compose and inject content into markdown files",
	Long: `Narrator manages README sections using <!-- sf:<name> --> markers.

Compose badges, shields, text, and other modules into managed sections.
Content between markers is owned by StageFreight and replaced on each run.
Everything outside markers is never touched.`,
}

func init() {
	rootCmd.AddCommand(narratorCmd)
}
