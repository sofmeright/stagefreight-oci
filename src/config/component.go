package config

// ComponentConfig holds GitLab CI component management configuration.
type ComponentConfig struct {
	// SpecFiles lists paths to component spec YAML files (templates/).
	// Used by `component docs` to generate input documentation.
	SpecFiles []string `yaml:"spec_files"`

	// Readme controls where generated docs are injected.
	Readme ReadmeConfig `yaml:"readme"`

	// Catalog enables adding a GitLab Catalog link to releases.
	Catalog bool `yaml:"catalog"`
}

// ReadmeConfig controls README documentation injection.
type ReadmeConfig struct {
	File    string `yaml:"file"`    // target README file (default: README.md)
	Section string `yaml:"section"` // sf:section name (default: "component-inputs")
	Branch  string `yaml:"branch"`  // branch to commit to (default: main)
}

// DefaultComponentConfig returns sensible defaults for component management.
func DefaultComponentConfig() ComponentConfig {
	return ComponentConfig{
		Readme: ReadmeConfig{
			File:    "README.md",
			Section: "component-inputs",
			Branch:  "main",
		},
		Catalog: true,
	}
}
