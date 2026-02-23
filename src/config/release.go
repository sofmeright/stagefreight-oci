package config

// ReleaseConfig holds release creation and sync configuration.
type ReleaseConfig struct {
	// Badge configures the release badge SVG commit.
	Badge BadgeConfig `yaml:"badge"`

	// Sync defines targets for cross-platform release sync.
	Sync []SyncTarget `yaml:"sync"`

	// Tags are tag templates resolved against git version info.
	// When configured, `release create` creates additional rolling tags
	// (e.g., "latest", "{major}.{minor}") pointing to the same commit.
	// Uses the same template syntax as docker registry tags:
	//   ["{version}", "{major}.{minor}", "latest"]
	Tags []string `yaml:"tags"`

	// GitTags controls which git tags trigger release auto-tagging.
	// Uses standard pattern syntax: regex, literal, or !negated.
	// Empty = all tags. Examples:
	//   ["^\\d+\\.\\d+\\.\\d+$"]  — stable semver only
	//   ["!^.*-rc"]               — exclude release candidates
	GitTags []string `yaml:"git_tags"`

	// Retention controls cleanup of old releases after a successful create.
	// Policies are additive (restic-style): a release is kept if ANY policy wants it.
	// Uses the same RetentionPolicy as docker tag retention.
	//
	// Accepts either a plain integer (shorthand for keep_last) or a policy map:
	//
	//   retention: 10                  # keep last 10 releases
	//
	//   retention:                     # restic-style policy
	//     keep_last: 3
	//     keep_monthly: 6
	//
	// Zero/empty = no cleanup.
	Retention RetentionPolicy `yaml:"retention,omitempty"`
}

// BadgeConfig controls the release status badge.
type BadgeConfig struct {
	Enabled bool   `yaml:"enabled"` // commit badge SVG to repo (default: true)
	Path    string `yaml:"path"`    // repo path for badge file (default: .badges/release.svg)
	Branch  string `yaml:"branch"`  // branch to commit badge to (default: main)
}

// SyncTarget defines a forge to sync releases, badges, and scan artifacts to.
type SyncTarget struct {
	// Name is a human-readable label for this sync target.
	Name string `yaml:"name"`

	// Provider is the forge type: gitlab, github, gitea.
	Provider string `yaml:"provider"`

	// URL is the forge base URL (e.g., "https://github.com").
	URL string `yaml:"url"`

	// Credentials is the env var prefix for authentication.
	// e.g., "GITHUB_SYNC" → GITHUB_SYNC_TOKEN env var.
	Credentials string `yaml:"credentials"`

	// ProjectID is the project identifier (e.g., "owner/repo" or numeric ID).
	// Can also be resolved from env: CI_PROJECT_ID, GITHUB_REPOSITORY.
	ProjectID string `yaml:"project_id"`

	// Branches controls when this target is synced.
	// Uses standard pattern syntax: regex, literal, or !negated.
	// Empty = always sync. Examples:
	//   ["^main$"]               — only sync on main
	//   ["!^develop$"]           — sync everything except develop
	//   ["^main$", "!^.*-wip$"]  — main, excluding wip suffixes
	Branches []string `yaml:"branches"`

	// Tags controls which tags trigger sync to this target.
	// Uses standard pattern syntax. Empty = all tags.
	// Examples:
	//   ["^v\\d+\\.\\d+\\.\\d+$"]  — stable semver only
	//   ["!^v.*-rc"]                — exclude release candidates
	Tags []string `yaml:"tags"`

	// SyncRelease syncs release notes and tags (default: true).
	SyncRelease bool `yaml:"sync_release"`

	// SyncAssets syncs scan artifacts (SARIF, SBOM) to the release (default: true).
	SyncAssets bool `yaml:"sync_assets"`

	// SyncBadge commits the badge SVG to this target (default: false).
	SyncBadge bool `yaml:"sync_badge"`
}

// DefaultReleaseConfig returns sensible defaults for release management.
func DefaultReleaseConfig() ReleaseConfig {
	return ReleaseConfig{
		Badge: BadgeConfig{
			Enabled: true,
			Path:    ".badges/release.svg",
			Branch:  "main",
		},
	}
}
