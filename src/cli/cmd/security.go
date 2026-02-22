package cmd

import (
	"github.com/spf13/cobra"
)

var securityCmd = &cobra.Command{
	Use:   "security",
	Short: "Security scanning commands",
	Long:  "Vulnerability scanning, SBOM generation, and security attestation.",
}

func init() {
	rootCmd.AddCommand(securityCmd)
}
