package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"
)

const defaultConfigFile = ".stagefreight.yml"

// Config is the top-level StageFreight configuration.
type Config struct {
	Sources         SourcesConfig         `yaml:"sources"`
	Git             GitConfig             `yaml:"git"`
	Lint            LintConfig            `yaml:"lint"`
	Docker          DockerConfig          `yaml:"docker"`
	Security        SecurityConfig        `yaml:"security"`
	Release         ReleaseConfig         `yaml:"release"`
	GitlabComponent GitlabComponentConfig `yaml:"gitlab_component"`
}

// Load reads configuration from a YAML file.
// If path is empty, it tries the default file.
// Returns sensible defaults if the file doesn't exist.
// Discards validation warnings; use LoadWithWarnings for full diagnostics.
func Load(path string) (*Config, error) {
	cfg, _, err := LoadWithWarnings(path)
	return cfg, err
}

// LoadWithWarnings reads configuration from a YAML file and returns
// validation warnings alongside the config. Warnings include deprecation
// notices (e.g., file→output alias) and typo detection.
func LoadWithWarnings(path string) (*Config, []string, error) {
	if path == "" {
		path = defaultConfigFile
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return defaults(), nil, nil
		}
		return nil, nil, err
	}

	cfg := defaults()
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		return nil, nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	// Apply deprecation aliases before validation (e.g., file→output for badges with gen fields)
	aliasWarnings := applyDeprecationAliases(cfg)

	warnings, verr := Validate(cfg)
	warnings = append(aliasWarnings, warnings...)
	if verr != nil {
		return nil, warnings, fmt.Errorf("validating %s: %w", path, verr)
	}

	return cfg, warnings, nil
}

func defaults() *Config {
	return &Config{
		Sources:         DefaultSourcesConfig(),
		Git:             DefaultGitConfig(),
		Lint:            DefaultLintConfig(),
		Docker:          DefaultDockerConfig(),
		Security:        DefaultSecurityConfig(),
		Release:         DefaultReleaseConfig(),
		GitlabComponent: DefaultGitlabComponentConfig(),
	}
}
