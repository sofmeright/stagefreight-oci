package config

// DockerConfig holds docker build configuration.
type DockerConfig struct {
	Context    string             `yaml:"context"`
	Dockerfile string             `yaml:"dockerfile"`
	Target     string             `yaml:"target"`
	Platforms  []string           `yaml:"platforms"`
	BuildArgs  map[string]string  `yaml:"build_args"`
	Registries []RegistryConfig   `yaml:"registries"`
	Cache      CacheConfig        `yaml:"cache"`
	Readme     DockerReadmeConfig `yaml:"readme"`
}

// DockerReadmeConfig controls README sync to container registries.
// This is a destination sync policy — it adapts canonical content (composed by
// git.narrator) for registry constraints. It must NOT choose sections/modules
// or compose badges.
type DockerReadmeConfig struct {
	Enabled     *bool           `yaml:"enabled"`
	File        string          `yaml:"file"`
	Description string          `yaml:"description"`
	LinkBase    string          `yaml:"link_base"`
	RawBase     string          `yaml:"raw_base"`
	Markers     *bool           `yaml:"markers"`
	StartMarker string          `yaml:"start_marker"`
	EndMarker   string          `yaml:"end_marker"`
	Transforms  []TransformRule `yaml:"transforms"`
}

// TransformRule defines a regex find/replace applied to README content.
type TransformRule struct {
	Pattern string `yaml:"pattern"`
	Replace string `yaml:"replace"`
}

// IsActive returns true if readme sync is explicitly enabled or any field is set.
func (r DockerReadmeConfig) IsActive() bool {
	if r.Enabled != nil {
		return *r.Enabled
	}
	return r.File != "" || r.Description != "" || r.LinkBase != "" ||
		r.Markers != nil || r.StartMarker != "" || r.EndMarker != "" ||
		len(r.Transforms) > 0
}

// RegistryConfig defines a registry push target.
type RegistryConfig struct {
	URL         string   `yaml:"url"`
	Path        string   `yaml:"path"`
	Tags        []string `yaml:"tags"`
	Credentials string   `yaml:"credentials"` // env var prefix for auth (e.g., "DOCKERHUB" → DOCKERHUB_USER/DOCKERHUB_PASS)
	Provider    string   `yaml:"provider"`    // registry vendor: dockerhub, ghcr, gitlab, jfrog, harbor, quay, gitea, generic
	Description string   `yaml:"description"` // per-registry short description override for readme sync

	// Branches controls which branches push to this registry.
	// Uses standard pattern syntax: regex, literal, or !negated.
	// Supports policy name resolution (e.g., "main" → resolved from git.policy.branches).
	// Empty = always push.
	Branches []string `yaml:"branches"`

	// GitTags controls which git tags trigger a push to this registry.
	// Uses standard pattern syntax: regex, literal, or !negated.
	// Supports policy name resolution (e.g., "stable" → resolved from git.policy.tags).
	// Empty = all tags (no filter).
	GitTags []string `yaml:"git_tags"`

	// Retention controls cleanup of old tags after a successful push.
	// Policies are additive (restic-style): a tag is kept if ANY policy wants it.
	//
	// Accepts either a plain integer (shorthand for keep_last) or a policy map:
	//
	//   retention: 10                  # keep last 10 tags
	//
	//   retention:                     # restic-style policy
	//     keep_last: 3
	//     keep_daily: 7
	//     keep_weekly: 4
	//     keep_monthly: 6
	//     keep_yearly: 2
	//
	// Zero/empty = no cleanup.
	Retention RetentionPolicy `yaml:"retention,omitempty"`
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
