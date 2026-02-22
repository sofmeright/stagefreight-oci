package config

// Condition is the universal conditional rule primitive used across StageFreight.
// Every feature that has tag/branch-sensitive behavior uses this same structure.
//
// Pattern syntax (applies to Tag, Branch, and any future filter fields):
//
//	"^main$"             — regex match (default)
//	"!^feature/.*"       — negated regex (! prefix)
//	"main"               — literal match (no regex metacharacters)
//	"!develop"           — negated literal
//
// Matching logic:
//   - Tag: tested against CI_COMMIT_TAG / git tag. Only evaluated when a tag is present.
//   - Branch: tested against CI_COMMIT_BRANCH / git branch.
//   - Multiple fields set: AND — all present fields must match.
//   - No fields set: catch-all (always matches).
//
// Rules are always evaluated top-down, first match wins. CLI overrides take precedence.
type Condition struct {
	// Tag is a pattern matched against the git tag (CI_COMMIT_TAG).
	// Only evaluated when a tag is present. Prefix with ! to negate.
	Tag string `yaml:"tag,omitempty"`

	// Branch is a pattern matched against the git branch (CI_COMMIT_BRANCH).
	// Prefix with ! to negate.
	Branch string `yaml:"branch,omitempty"`
}
