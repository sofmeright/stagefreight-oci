package config

import (
	"os"
	"regexp"
	"strings"
)

// MatchCondition evaluates a Condition against the current CI environment.
// Returns true if all present fields match (AND logic).
// Empty condition = catch-all (always true).
func MatchCondition(c Condition) bool {
	tag := os.Getenv("CI_COMMIT_TAG")
	branch := os.Getenv("CI_COMMIT_BRANCH")
	if branch == "" {
		branch = os.Getenv("GITHUB_REF_NAME")
	}

	return MatchConditionWith(c, tag, branch)
}

// MatchConditionWith evaluates a Condition against explicit tag/branch values.
// Use this when you already have the values resolved (e.g., from git detection).
func MatchConditionWith(c Condition, tag, branch string) bool {
	if c.Tag != "" {
		// Tag rule present but no tag in env → no match
		if tag == "" {
			return false
		}
		if !matchPattern(c.Tag, tag) {
			return false
		}
	}

	if c.Branch != "" {
		if !matchPattern(c.Branch, branch) {
			return false
		}
	}

	return true
}

// matchPattern evaluates a single pattern against a value.
//
// Syntax:
//
//	"^main$"         — regex match
//	"!^feature/.*"   — negated regex
//	"main"           — treated as regex (anchored or not, user decides)
//	"!develop"       — negated match
func matchPattern(pattern, value string) bool {
	negate := false
	if strings.HasPrefix(pattern, "!") {
		negate = true
		pattern = pattern[1:]
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		// Invalid regex → treat as literal
		matched := pattern == value
		if negate {
			return !matched
		}
		return matched
	}

	matched := re.MatchString(value)
	if negate {
		return !matched
	}
	return matched
}

// MatchPatterns evaluates a list of patterns against a value (OR logic).
// Any pattern matching means the value is allowed.
// Empty list = always allowed (no filter).
// Supports ! negation — a negated pattern excludes even if others include.
//
// Evaluation: exclude patterns (!) are checked first. If any exclude matches,
// the value is rejected. Then include patterns are checked — if any matches,
// the value is allowed. If only exclude patterns exist and none matched,
// the value is allowed (exclude-only = allowlist by negation).
func MatchPatterns(patterns []string, value string) bool {
	if len(patterns) == 0 {
		return true
	}

	var includes []string
	var excludes []string
	for _, p := range patterns {
		if strings.HasPrefix(p, "!") {
			excludes = append(excludes, p[1:])
		} else {
			includes = append(includes, p)
		}
	}

	// Excludes checked first — any exclude match = rejected
	for _, p := range excludes {
		re, err := regexp.Compile(p)
		if err != nil {
			if p == value {
				return false
			}
			continue
		}
		if re.MatchString(value) {
			return false
		}
	}

	// No includes = exclude-only mode (everything not excluded is allowed)
	if len(includes) == 0 {
		return true
	}

	// Any include match = allowed
	for _, p := range includes {
		re, err := regexp.Compile(p)
		if err != nil {
			if p == value {
				return true
			}
			continue
		}
		if re.MatchString(value) {
			return true
		}
	}

	return false
}
