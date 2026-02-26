package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/sofmeright/stagefreight/src/output"
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

	color := output.UseColor()
	w := os.Stdout

	ctx := context.Background()
	start := time.Now()
	result, err := security.Scan(ctx, scanCfg)
	elapsed := time.Since(start)

	if err != nil {
		return fmt.Errorf("security scan: %w", err)
	}

	// Collect artifacts
	artifacts := append([]string{}, result.Artifacts...)

	// Resolve detail level from rules (CLI override > tag/branch rules > default)
	detail := security.ResolveDetailLevel(cfg.Security, secScanDetail, cfg.Git.Policy)

	// Build and write summary
	summary := security.BuildSummary(result, detail)
	var summaryPath string
	if summary != "" {
		summaryPath = scanCfg.OutputDir + "/summary.md"
		if wErr := os.WriteFile(summaryPath, []byte(summary), 0o644); wErr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not write summary: %v\n", wErr)
			summaryPath = ""
		} else {
			artifacts = append(artifacts, fmt.Sprintf("%s (detail: %s)", summaryPath, detail))
		}
	}

	// Determine status
	var status, statusDetail string
	switch result.Status {
	case "passed":
		status = "success"
		statusDetail = "passed"
	case "warning":
		status = "skipped" // yellow icon
		statusDetail = fmt.Sprintf("%d high vulnerabilities", result.High)
	case "critical":
		status = "failed"
		statusDetail = fmt.Sprintf("%d critical vulnerabilities", result.Critical)
	default:
		status = "success"
		statusDetail = result.Status
	}

	// ── Security Scan section ──
	output.SectionStart(w, "sf_security", "Security Scan")
	sec := output.NewSection(w, "Security Scan", elapsed, color)
	sec.Row("%-16s%s", "image", imageRef)
	sec.Row("%-16s%s %s", "status", statusDetail, output.StatusIcon(status, color))

	if len(artifacts) > 0 {
		sec.Row("")
		sec.Row("artifacts")
		for _, a := range artifacts {
			sec.Row("  → %s", a)
		}
	}

	sec.Close()
	output.SectionEnd(w, "sf_security")

	// Print verbose summary to stdout
	if verbose && summary != "" {
		fmt.Println()
		fmt.Print(summary)
	}

	// Fail if configured and critical vulns found
	if scanCfg.FailOnCritical && result.Critical > 0 {
		return fmt.Errorf("security scan failed: %d critical vulnerabilities", result.Critical)
	}

	return nil
}
