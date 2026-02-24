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
	Readme     DockerReadmeConfig `yaml:"readme"`
}

// DockerReadmeConfig controls README sync to container registries.
type DockerReadmeConfig struct {
	Enabled     *bool           `yaml:"enabled"`
	File        string          `yaml:"file"`
	Description string          `yaml:"description"`
	LinkBase    string          `yaml:"link_base"`
	RawBase     string          `yaml:"raw_base"`
	Badges      []BadgeEntry    `yaml:"badges"`
	Markers     *bool           `yaml:"markers"`
	StartMarker string          `yaml:"start_marker"`
	EndMarker   string          `yaml:"end_marker"`
	Transforms  []TransformRule `yaml:"transforms"`
}

// BadgeEntry defines a single badge for narrator-driven injection into synced READMEs.
// Exactly one of File or URL must be set per entry.
type BadgeEntry struct {
	Alt     string `yaml:"alt"`     // image alt text
	File    string `yaml:"file"`    // relative path to committed SVG (resolved via raw_base)
	URL     string `yaml:"url"`     // absolute image URL (shields.io, etc.)
	Link    string `yaml:"link"`    // click target (absolute URL or relative path resolved via link_base)
	Section string `yaml:"section"` // target section name (default: "badges")
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
		len(r.Transforms) > 0 || len(r.Badges) > 0
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
