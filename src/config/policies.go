package config

// PoliciesConfig defines named regex patterns for git tag and branch matching.
// Policy names are referenced by target when conditions (e.g., git_tags: [stable])
// and resolved to regex patterns during evaluation.
//
// v2 promotes this from git.policy to a top-level key, reflecting that policies
// are routing primitives referenced across builds, targets, and releases.
type PoliciesConfig struct {
	// GitTags maps policy names to regex patterns for git tag matching.
	// e.g., "stable": "^\\d+\\.\\d+\\.\\d+$"
	// Named "git_tags" (not "tags") to avoid collision with Docker image tags.
	GitTags map[string]string `yaml:"git_tags"`

	// Branches maps policy names to regex patterns for branch matching.
	// e.g., "main": "^main$"
	Branches map[string]string `yaml:"branches"`
}

// DefaultPoliciesConfig returns an empty policies config.
func DefaultPoliciesConfig() PoliciesConfig {
	return PoliciesConfig{
		GitTags:  map[string]string{},
		Branches: map[string]string{},
	}
}
