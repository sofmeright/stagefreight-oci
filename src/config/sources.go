package config

// SourcesConfig holds the build source definitions.
// Minimal now; prepares for multi-source builds in the future.
type SourcesConfig struct {
	Primary SourceConfig `yaml:"primary"`
}

// SourceConfig defines a single build source.
type SourceConfig struct {
	Kind          string `yaml:"kind"`           // source type (default: "git")
	Worktree      string `yaml:"worktree"`       // path to working tree (default: ".")
	URL           string `yaml:"url"`            // optional: enables deterministic link_base/raw_base derivation
	DefaultBranch string `yaml:"default_branch"` // optional: used with URL for raw_base derivation
}

// DefaultSourcesConfig returns sensible defaults for source configuration.
func DefaultSourcesConfig() SourcesConfig {
	return SourcesConfig{
		Primary: SourceConfig{
			Kind:     "git",
			Worktree: ".",
		},
	}
}
