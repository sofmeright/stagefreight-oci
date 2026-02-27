package config

// BuildConfig defines a named build artifact. Each build has a unique ID
// (referenced by targets) and a kind that determines which fields are valid.
//
// This is a discriminated union keyed by Kind — only fields relevant to the
// kind should be set. Validated at load time by v2 validation.
type BuildConfig struct {
	// ID is the unique identifier for this build, referenced by targets.
	ID string `yaml:"id"`

	// Kind is the build type. Determines which fields are valid.
	// Supported: "docker". Future: "helm", "binary".
	Kind string `yaml:"kind"`

	// SelectTags enables CLI filtering via --select.
	SelectTags []string `yaml:"select_tags,omitempty"`

	// ── kind: docker ──────────────────────────────────────────────────────

	// Dockerfile is the path to the Dockerfile. Default: auto-detect.
	Dockerfile string `yaml:"dockerfile,omitempty"`

	// Context is the Docker build context path. Default: "." (repo root).
	Context string `yaml:"context,omitempty"`

	// Target is the --target stage name for multi-stage builds.
	Target string `yaml:"target,omitempty"`

	// Platforms lists the target platforms. Default: [linux/{current_arch}].
	Platforms []string `yaml:"platforms,omitempty"`

	// BuildArgs are key-value pairs passed as --build-arg. Supports templates.
	BuildArgs map[string]string `yaml:"build_args,omitempty"`

	// Cache holds build cache settings.
	Cache CacheConfig `yaml:"cache,omitempty"`
}
