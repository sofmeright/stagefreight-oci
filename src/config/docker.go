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
	Credentials string   `yaml:"credentials"` // env var prefix for auth (e.g., "DOCKERHUB" → DOCKERHUB_USER/DOCKERHUB_PASS)
	Provider    string   `yaml:"provider"`    // registry vendor: dockerhub, ghcr, gitlab, jfrog, harbor, quay, generic

	// Branches controls which branches push to this registry.
	// Uses standard pattern syntax: regex, literal, or !negated.
	// Empty = always push. Examples:
	//   ["^main$"]                    — only main
	//   ["^main$", "^release/.*"]     — main or release branches
	//   ["!^develop$"]                — everything except develop
	//   ["^main$", "!^.*-wip$"]       — main, but not if it ends in -wip
	Branches []string `yaml:"branches"`

	// GitTags controls which git tags trigger a push to this registry.
	// Uses standard pattern syntax: regex, literal, or !negated.
	// Empty = all tags (no filter). Examples:
	//   ["^v\\d+\\.\\d+\\.\\d+$"]     — stable semver only
	//   ["!^v.*-rc"]                   — exclude release candidates
	//   ["^v.*", "!^v.*-alpha"]        — all v-prefixed except alpha
	GitTags []string `yaml:"git_tags"`

	// Retention sets a maximum number of tags to keep for this registry.
	// After a successful push, StageFreight prunes older tags matching the
	// same tag patterns, keeping only the most recent N. Zero = no cleanup.
	// Requires registry API support (Quay, Harbor, GitLab, JFrog, GHCR).
	// Docker Hub does not expose a tag deletion API.
	Retention int `yaml:"retention,omitempty"`
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
