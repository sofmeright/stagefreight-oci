package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/sofmeright/stagefreight/src/security"
)

var (
	secScanImage      string
	secScanOutputDir  string
	secScanSBOM       bool
	secScanFailCrit   bool
	secScanSkip       bool
	secScanDetail     string
)

var securityScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Run vulnerability scan and generate SBOM",
	Long: `Scan a container image for vulnerabilities using Trivy
and optionally generate SBOM artifacts using Syft.

Results are written to the output directory as JSON, SARIF, and SBOM files.
A markdown summary is generated at the configured detail level for embedding
in release notes.`,
	RunE: runSecurityScan,
}

func init() {
	securityScanCmd.Flags().StringVar(&secScanImage, "image", "", "image reference or tarball to scan (required)")
	securityScanCmd.Flags().StringVarP(&secScanOutputDir, "output", "o", "", "output directory for artifacts (default: from config)")
	securityScanCmd.Flags().BoolVar(&secScanSBOM, "sbom", true, "generate SBOM artifacts")
	securityScanCmd.Flags().BoolVar(&secScanFailCrit, "fail-on-critical", false, "exit non-zero if critical vulnerabilities found")
	securityScanCmd.Flags().BoolVar(&secScanSkip, "skip", false, "skip scan (for pipeline control)")
	securityScanCmd.Flags().StringVar(&secScanDetail, "security-detail", "", "override detail level for summary: none, counts, detailed, full")

	securityCmd.AddCommand(securityScanCmd)
}

func runSecurityScan(cmd *cobra.Command, args []string) error {
	if secScanSkip {
		fmt.Println("  security scan skipped")
		return nil
	}

	imageRef := secScanImage
	if imageRef == "" && len(args) > 0 {
		imageRef = args[0]
	}
	if imageRef == "" {
		return fmt.Errorf("--image is required (or pass image ref as argument)")
	}

	// Merge CLI flags with config defaults
	scanCfg := security.ScanConfig{
		Enabled:        !secScanSkip,
		SBOMEnabled:    secScanSBOM,
		FailOnCritical: secScanFailCrit || cfg.Security.FailOnCritical,
		ImageRef:       imageRef,
		OutputDir:      secScanOutputDir,
	}
	if scanCfg.OutputDir == "" {
		scanCfg.OutputDir = cfg.Security.OutputDir
	}

	// Ensure output directory exists
	if err := os.MkdirAll(scanCfg.OutputDir, 0o755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	ctx := context.Background()
	result, err := security.Scan(ctx, scanCfg)
	if err != nil {
		return fmt.Errorf("security scan: %w", err)
	}

	// Report
	switch result.Status {
	case "passed":
		fmt.Printf("  security %s (no critical or high vulnerabilities)\n", colorGreen("✓"))
	case "warning":
		fmt.Printf("  security %s (%d high vulnerabilities)\n", colorYellow("⚠"), result.High)
	case "critical":
		fmt.Printf("  security %s (%d critical vulnerabilities)\n", colorRed("✗"), result.Critical)
	case "skipped":
		fmt.Println("  security scan skipped")
		return nil
	}

	// Print artifact paths
	for _, a := range result.Artifacts {
		fmt.Printf("  → %s\n", a)
	}

	// Resolve detail level from rules (CLI override > tag/branch rules > default)
	detail := security.ResolveDetailLevel(cfg.Security, secScanDetail, cfg.Git.Policy)

	// Build and write summary
	summary := security.BuildSummary(result, detail)
	if summary != "" {
		// Write summary to file for downstream consumption by release notes
		summaryPath := scanCfg.OutputDir + "/summary.md"
		if err := os.WriteFile(summaryPath, []byte(summary), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: could not write summary: %v\n", err)
		} else {
			fmt.Printf("  → %s (detail: %s)\n", summaryPath, detail)
		}

		// Print to stdout if verbose
		if verbose {
			fmt.Println()
			fmt.Print(summary)
		}
	}

	// Fail if configured and critical vulns found
	if scanCfg.FailOnCritical && result.Critical > 0 {
		return fmt.Errorf("security scan failed: %d critical vulnerabilities", result.Critical)
	}

	return nil
}

func colorYellow(s string) string {
	return "\033[33m" + s + "\033[0m"
}
