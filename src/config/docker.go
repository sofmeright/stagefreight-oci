package config

// DockerConfig holds docker build configuration.
type DockerConfig struct {
	Context    string            `yaml:"context"`
	Dockerfile string            `yaml:"dockerfile"`
	Target     string            `yaml:"target"`
	Platforms  []string          `yaml:"platforms"`
	BuildArgs  map[string]string `yaml:"build_args"`
	Registries []RegistryConfig  `yaml:"registries"`
	Cache      CacheConfig       `yaml:"cache"`
}

// RegistryConfig defines a registry push target.
type RegistryConfig struct {
	URL         string   `yaml:"url"`
	Path        string   `yaml:"path"`
	Tags        []string `yaml:"tags"`
	Branches    []string `yaml:"branches"`
	Credentials string   `yaml:"credentials"`
}

// CacheConfig holds build cache settings.
type CacheConfig struct {
	Watch      []WatchRule `yaml:"watch"`
	AutoDetect *bool       `yaml:"auto_detect"`
}

// WatchRule defines a cache-busting rule.
type WatchRule struct {
	Paths       []string `yaml:"paths"`
	Invalidates []string `yaml:"invalidates"`
}

// DefaultDockerConfig returns sensible defaults for docker builds.
func DefaultDockerConfig() DockerConfig {
	return DockerConfig{
		Context:   ".",
		Platforms: []string{},
		BuildArgs: map[string]string{},
	}
}
