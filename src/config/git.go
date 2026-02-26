package config

// GitConfig holds git-related configuration: policy and narrator.
type GitConfig struct {
	Policy   GitPolicyConfig `yaml:"policy"`
	Narrator NarratorConfig  `yaml:"narrator"`
}

// GitPolicyConfig defines named patterns for git tag and branch matching.
// Policy names are referenced by registry, release, and sync configs
// (e.g., git_tags: [stable]) and resolved to regex patterns.
type GitPolicyConfig struct {
	Tags     map[string]string `yaml:"tags"`
	Branches map[string]string `yaml:"branches"`
}

// DefaultGitConfig returns sensible defaults for git configuration.
func DefaultGitConfig() GitConfig {
	return GitConfig{
		Policy: GitPolicyConfig{
			Tags:     map[string]string{},
			Branches: map[string]string{},
		},
	}
}
