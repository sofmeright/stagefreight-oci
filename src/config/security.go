package config

// SecurityConfig holds security scanning configuration.
type SecurityConfig struct {
	Enabled        bool   `yaml:"enabled"`          // run vulnerability scanning (default: true)
	SBOMEnabled    bool   `yaml:"sbom"`             // generate SBOM artifacts (default: true)
	FailOnCritical bool   `yaml:"fail_on_critical"` // fail the pipeline if critical vulns found
	OutputDir      string `yaml:"output_dir"`       // directory for scan artifacts (default: .stagefreight/security)

	// ReleaseDetail is the default detail level for security info in release notes.
	// Values: "none", "counts", "detailed", "full" (default: "counts").
	ReleaseDetail string `yaml:"release_detail"`

	// ReleaseDetailRules are conditional overrides evaluated top-down (first match wins).
	// Uses the standard Condition primitive for tag/branch matching with ! negation.
	ReleaseDetailRules []DetailRule `yaml:"release_detail_rules"`
}

// DetailRule is a conditional override for security detail level in release notes.
// Embeds Condition for standard tag/branch pattern matching.
type DetailRule struct {
	Condition `yaml:",inline"`

	// Detail is the detail level to use when this rule matches.
	// Values: "none", "counts", "detailed", "full".
	Detail string `yaml:"detail"`
}

// DefaultSecurityConfig returns sensible defaults for security scanning.
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		Enabled:        true,
		SBOMEnabled:    true,
		FailOnCritical: false,
		OutputDir:      ".stagefreight/security",
		ReleaseDetail:  "counts",
	}
}
