package config

import (
	"errors"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"
)

const defaultConfigFile = ".stagefreight.yml"

// Config is the top-level StageFreight configuration.
type Config struct {
	Lint      LintConfig      `yaml:"lint"`
	Docker    DockerConfig    `yaml:"docker"`
	Security  SecurityConfig  `yaml:"security"`
	Release   ReleaseConfig   `yaml:"release"`
	Component ComponentConfig `yaml:"component"`
	Badges    BadgesConfig    `yaml:"badges"`
	Narrator  NarratorConfig  `yaml:"narrator"`
}

// Load reads configuration from a YAML file.
// If path is empty, it tries the default file.
// Returns sensible defaults if the file doesn't exist.
func Load(path string) (*Config, error) {
	if path == "" {
		path = defaultConfigFile
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return defaults(), nil
		}
		return nil, err
	}

	cfg := defaults()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func defaults() *Config {
	return &Config{
		Lint:      DefaultLintConfig(),
		Docker:    DefaultDockerConfig(),
		Security:  DefaultSecurityConfig(),
		Release:   DefaultReleaseConfig(),
		Component: DefaultComponentConfig(),
		Badges:    DefaultBadgesConfig(),
		Narrator:  DefaultNarratorConfig(),
	}
}
