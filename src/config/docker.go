package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

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
	Provider    string   `yaml:"provider"`    // registry vendor: dockerhub, ghcr, gitlab, jfrog, harbor, quay, gitea, generic

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

// RetentionPolicy defines how many tags to keep using time-bucketed rules.
// Policies are additive — a tag survives if ANY rule wants to keep it.
// This mirrors restic's forget policy.
type RetentionPolicy struct {
	KeepLast    int `yaml:"keep_last"`    // keep the N most recent tags
	KeepDaily   int `yaml:"keep_daily"`   // keep one per day for the last N days
	KeepWeekly  int `yaml:"keep_weekly"`  // keep one per week for the last N weeks
	KeepMonthly int `yaml:"keep_monthly"` // keep one per month for the last N months
	KeepYearly  int `yaml:"keep_yearly"`  // keep one per year for the last N years
}

// Active returns true if any retention rule is configured.
func (r RetentionPolicy) Active() bool {
	return r.KeepLast > 0 || r.KeepDaily > 0 || r.KeepWeekly > 0 || r.KeepMonthly > 0 || r.KeepYearly > 0
}

// UnmarshalYAML implements custom unmarshaling so retention accepts both:
//
//	retention: 10          → RetentionPolicy{KeepLast: 10}
//	retention:
//	  keep_last: 3
//	  keep_daily: 7        → RetentionPolicy{KeepLast: 3, KeepDaily: 7}
func (r *RetentionPolicy) UnmarshalYAML(value *yaml.Node) error {
	// Try scalar (int) first
	if value.Kind == yaml.ScalarNode {
		var n int
		if err := value.Decode(&n); err != nil {
			return fmt.Errorf("retention: expected integer or policy map, got %q", value.Value)
		}
		r.KeepLast = n
		return nil
	}

	// Try map
	if value.Kind == yaml.MappingNode {
		// Decode into an alias type to avoid infinite recursion
		type policyAlias RetentionPolicy
		var alias policyAlias
		if err := value.Decode(&alias); err != nil {
			return fmt.Errorf("retention: %w", err)
		}
		*r = RetentionPolicy(alias)
		return nil
	}

	return fmt.Errorf("retention: expected integer or map, got YAML kind %d", value.Kind)
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
