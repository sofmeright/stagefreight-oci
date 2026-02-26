package config

// GitlabComponentConfig holds GitLab CI component management configuration.
type GitlabComponentConfig struct {
	// SpecFiles lists paths to component spec YAML files (templates/).
	// Used by `component docs` to generate input documentation.
	SpecFiles []string `yaml:"spec_files"`

	// Catalog enables adding a GitLab Catalog link to releases.
	Catalog bool `yaml:"catalog"`
}

// DefaultGitlabComponentConfig returns sensible defaults for component management.
func DefaultGitlabComponentConfig() GitlabComponentConfig {
	return GitlabComponentConfig{
		Catalog: true,
	}
}
