// Package security provides vulnerability scanning and SBOM generation.
// Orchestrates external tools (Trivy, Syft) and produces structured results
// that feed into release notes and forge uploads.
package security

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ScanConfig holds security scan configuration.
type ScanConfig struct {
	Enabled        bool   // run vulnerability scan
	SBOMEnabled    bool   // generate SBOM
	FailOnCritical bool   // fail if critical vulns found
	ImageRef       string // image reference or tarball path to scan
	OutputDir      string // directory for scan artifacts
}

// Vulnerability is a single parsed vulnerability from the scan.
type Vulnerability struct {
	ID          string // CVE ID (e.g., "CVE-2026-1234")
	Severity    string // CRITICAL, HIGH, MEDIUM, LOW
	Package     string // affected package name
	Installed   string // installed version
	FixedIn     string // version that fixes the vuln
	Description string // one-line description
}

// ScanResult holds the outcome of a security scan.
type ScanResult struct {
	Critical        int             // count of critical vulnerabilities
	High            int             // count of high vulnerabilities
	Medium          int             // count of medium vulnerabilities
	Low             int             // count of low vulnerabilities
	Vulnerabilities []Vulnerability // parsed vulnerability details (for detailed/full output)
	Status          string          // "passed", "warning", "critical"
	Artifacts       []string        // paths to generated files (JSON, SARIF, SBOM)
	Summary         string          // markdown summary for embedding in release notes
	EngineVersion   string          // best-effort: from `trivy --version` or empty
	OS              string          // "alpine 3.21.3" (from Trivy JSON Metadata.OS)
}

// Scan runs a Trivy vulnerability scan and optionally generates SBOMs.
func Scan(ctx context.Context, cfg ScanConfig) (*ScanResult, error) {
	if !cfg.Enabled {
		return &ScanResult{Status: "skipped"}, nil
	}

	result := &ScanResult{}

	if cfg.OutputDir == "" {
		cfg.OutputDir = "."
	}

	// Best-effort engine version (silent capture, no stdout/stderr connection).
	if out, verErr := exec.Command("trivy", "--version").Output(); verErr == nil {
		for _, ln := range strings.Split(string(out), "\n") {
			ln = strings.TrimSpace(strings.TrimRight(ln, "\r"))
			if ln == "" {
				continue
			}
			// Find a semver-ish token (N.N.N) in the first non-empty line.
			for _, tok := range strings.Fields(ln) {
				t := strings.TrimPrefix(tok, "v")
				if strings.Count(t, ".") >= 2 && len(t) >= 5 {
					result.EngineVersion = "Trivy " + t
					break
				}
			}
			break
		}
	}

	// Run Trivy JSON scan
	jsonPath := cfg.OutputDir + "/security-scan.json"
	if err := runTrivy(ctx, cfg.ImageRef, "json", jsonPath); err != nil {
		return nil, fmt.Errorf("trivy scan: %w", err)
	}
	result.Artifacts = append(result.Artifacts, jsonPath)

	// Run Trivy SARIF scan
	sarifPath := cfg.OutputDir + "/vulnerability-report.sarif"
	if err := runTrivy(ctx, cfg.ImageRef, "sarif", sarifPath); err != nil {
		return nil, fmt.Errorf("trivy sarif: %w", err)
	}
	result.Artifacts = append(result.Artifacts, sarifPath)

	// Parse vulnerabilities from JSON (full detail, not just counts)
	if err := parseVulnerabilities(jsonPath, result); err != nil {
		return nil, fmt.Errorf("parsing scan results: %w", err)
	}

	// Determine status
	switch {
	case result.Critical > 0:
		result.Status = "critical"
	case result.High > 0:
		result.Status = "warning"
	default:
		result.Status = "passed"
	}

	// Generate SBOM if enabled
	if cfg.SBOMEnabled {
		spdxPath := cfg.OutputDir + "/sbom.spdx.json"
		if err := runSyft(ctx, cfg.ImageRef, "spdx-json", spdxPath); err != nil {
			return nil, fmt.Errorf("syft spdx: %w", err)
		}
		result.Artifacts = append(result.Artifacts, spdxPath)

		cdxPath := cfg.OutputDir + "/sbom.cyclonedx.json"
		if err := runSyft(ctx, cfg.ImageRef, "cyclonedx-json", cdxPath); err != nil {
			return nil, fmt.Errorf("syft cyclonedx: %w", err)
		}
		result.Artifacts = append(result.Artifacts, cdxPath)
	}

	return result, nil
}

// BuildSummary generates a markdown summary at the specified detail level.
// Returns (tile, body):
//   - tile: single-line status for hero area (e.g., "🛡️ ✅ **Passed** — no critical or high vulnerabilities")
//   - body: full section content (status line + optional <details> block with CVE data)
//
// Detail levels: "none", "counts", "detailed", "full".
func BuildSummary(result *ScanResult, detail string) (tile, body string) {
	if result.Status == "skipped" || detail == "none" {
		return "", ""
	}

	tile = buildStatusTile(result)

	switch detail {
	case "full":
		body = buildFullBody(result, tile)
	case "detailed":
		body = buildDetailedBody(result, tile)
	default: // "counts" or unrecognized
		body = tile + "\n"
	}
	return tile, body
}

// buildStatusTile produces the one-line security status.
func buildStatusTile(result *ScanResult) string {
	return fmt.Sprintf("🛡️ %s — %s", statusEmoji(result.Status), statusDetail(result))
}

func statusEmoji(status string) string {
	switch status {
	case "passed":
		return "✅ **Passed**"
	case "warning":
		return "⚠️ **Warning**"
	case "critical":
		return "❌ **Critical**"
	case "skipped":
		return "⏭️ **Skipped**"
	default:
		return status
	}
}

func statusDetail(result *ScanResult) string {
	total := result.Critical + result.High + result.Medium + result.Low
	if total == 0 {
		return "no vulnerabilities found"
	}
	switch {
	case result.Critical > 0 && result.High > 0:
		return fmt.Sprintf("%d critical and %d high vulnerabilities detected", result.Critical, result.High)
	case result.Critical > 0:
		return fmt.Sprintf("%d critical vulnerabilities detected", result.Critical)
	case result.High > 0:
		return fmt.Sprintf("%d high vulnerabilities detected", result.High)
	default:
		return fmt.Sprintf("%d vulnerabilities (%d medium, %d low)", total, result.Medium, result.Low)
	}
}

// vulnCountsSuffix builds a compact counts string for <summary> tags.
// Only includes non-zero severities.
func vulnCountsSuffix(result *ScanResult) string {
	var parts []string
	if result.Critical > 0 {
		parts = append(parts, fmt.Sprintf("%d critical", result.Critical))
	}
	if result.High > 0 {
		parts = append(parts, fmt.Sprintf("%d high", result.High))
	}
	if result.Medium > 0 {
		parts = append(parts, fmt.Sprintf("%d medium", result.Medium))
	}
	if result.Low > 0 {
		parts = append(parts, fmt.Sprintf("%d low", result.Low))
	}
	if len(parts) == 0 {
		return ""
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func buildDetailedBody(result *ScanResult, tile string) string {
	var b strings.Builder
	b.WriteString(tile)
	b.WriteString("\n")

	total := result.Critical + result.High + result.Medium + result.Low
	if total == 0 {
		return b.String()
	}

	// Collapsible CVE lists for critical and high
	b.WriteString(fmt.Sprintf("\n<details>\n<summary>Vulnerability details %s</summary>\n", vulnCountsSuffix(result)))

	maxPerSeverity := 5
	for _, sev := range []string{"CRITICAL", "HIGH"} {
		vulns := filterBySeverity(result.Vulnerabilities, sev)
		if len(vulns) == 0 {
			continue
		}

		b.WriteString(fmt.Sprintf("\n#### %s Vulnerabilities\n", titleCase(sev)))
		shown := 0
		for _, v := range vulns {
			if shown >= maxPerSeverity {
				remaining := len(vulns) - maxPerSeverity
				b.WriteString(fmt.Sprintf("- ... and %d more (see full report in release assets)\n", remaining))
				break
			}
			desc := v.Description
			if len(desc) > 80 {
				desc = desc[:77] + "..."
			}
			b.WriteString(fmt.Sprintf("- **%s** — %s (%s)\n", v.ID, desc, v.Package))
			shown++
		}
	}

	b.WriteString("\n</details>\n")
	return b.String()
}

func buildFullBody(result *ScanResult, tile string) string {
	var b strings.Builder
	b.WriteString(tile)
	b.WriteString("\n")

	total := result.Critical + result.High + result.Medium + result.Low
	if total == 0 {
		return b.String()
	}

	b.WriteString(fmt.Sprintf("\n<details>\n<summary>Vulnerability details %s</summary>\n\n", vulnCountsSuffix(result)))

	b.WriteString("| Severity | CVE | Package | Installed | Fixed | Description |\n")
	b.WriteString("|---|---|---|---|---|---|\n")

	for _, sev := range []string{"CRITICAL", "HIGH", "MEDIUM", "LOW"} {
		vulns := filterBySeverity(result.Vulnerabilities, sev)
		for _, v := range vulns {
			sevDisplay := titleCase(sev)
			if sev == "CRITICAL" {
				sevDisplay = "**Critical**"
			}
			desc := v.Description
			if len(desc) > 60 {
				desc = desc[:57] + "..."
			}
			fixedIn := v.FixedIn
			if fixedIn == "" {
				fixedIn = "—"
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
				sevDisplay, v.ID, v.Package, v.Installed, fixedIn, desc))
		}
	}

	b.WriteString("\n</details>\n")
	return b.String()
}

func filterBySeverity(vulns []Vulnerability, severity string) []Vulnerability {
	var out []Vulnerability
	for _, v := range vulns {
		if strings.EqualFold(v.Severity, severity) {
			out = append(out, v)
		}
	}
	return out
}

func titleCase(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

func runTrivy(ctx context.Context, imageRef, format, output string) error {
	args := []string{"image", "--format", format, "--output", output, imageRef}
	cmd := exec.CommandContext(ctx, "trivy", args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runSyft(ctx context.Context, imageRef, format, output string) error {
	cmd := exec.CommandContext(ctx, "syft", imageRef, "-o", format)
	outFile, err := os.Create(output)
	if err != nil {
		return err
	}
	defer outFile.Close()
	cmd.Stdout = outFile
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func parseVulnerabilities(jsonPath string, result *ScanResult) error {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return err
	}

	// Trivy JSON structure
	var report struct {
		Metadata struct {
			OS struct {
				Family string `json:"Family"`
				Name   string `json:"Name"`
			} `json:"OS"`
		} `json:"Metadata"`
		Results []struct {
			Vulnerabilities []struct {
				VulnerabilityID  string `json:"VulnerabilityID"`
				Severity         string `json:"Severity"`
				PkgName          string `json:"PkgName"`
				InstalledVersion string `json:"InstalledVersion"`
				FixedVersion     string `json:"FixedVersion"`
				Title            string `json:"Title"`
				Description      string `json:"Description"`
			} `json:"Vulnerabilities"`
		} `json:"Results"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return err
	}

	// Extract OS metadata (best-effort).
	family := strings.TrimSpace(report.Metadata.OS.Family)
	name := strings.TrimSpace(report.Metadata.OS.Name)
	if family != "" && name != "" {
		result.OS = family + " " + name
	} else if family != "" {
		result.OS = family
	} else if name != "" {
		result.OS = name
	}

	for _, r := range report.Results {
		for _, v := range r.Vulnerabilities {
			sev := strings.ToUpper(v.Severity)
			switch sev {
			case "CRITICAL":
				result.Critical++
			case "HIGH":
				result.High++
			case "MEDIUM":
				result.Medium++
			case "LOW":
				result.Low++
			}

			// Use Title if available, fall back to truncated Description
			desc := v.Title
			if desc == "" && v.Description != "" {
				desc = v.Description
				if len(desc) > 100 {
					desc = desc[:97] + "..."
				}
			}

			result.Vulnerabilities = append(result.Vulnerabilities, Vulnerability{
				ID:          v.VulnerabilityID,
				Severity:    sev,
				Package:     v.PkgName,
				Installed:   v.InstalledVersion,
				FixedIn:     v.FixedVersion,
				Description: desc,
			})
		}
	}
	return nil
}
