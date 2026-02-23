package registry

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/sofmeright/stagefreight/src/config"
)

// RetentionResult captures what the retention engine did.
type RetentionResult struct {
	Provider string
	Repo     string
	Matched  int      // tags matching the pattern set
	Kept     int      // tags kept by policy
	Deleted  []string // tags successfully deleted
	Errors   []error  // errors from individual deletes
}

// ApplyRetention lists all tags on the registry, filters them by the given
// tag patterns (using config.MatchPatterns with full !/OR/AND semantics),
// sorts by creation time descending, and applies restic-style retention
// policies to decide which tags to keep.
//
// Policies are additive — a tag survives if ANY policy wants to keep it:
//   - keep_last N:    keep the N most recent
//   - keep_daily N:   keep one per day for the last N days
//   - keep_weekly N:  keep one per week for the last N weeks
//   - keep_monthly N: keep one per month for the last N months
//   - keep_yearly N:  keep one per year for the last N years
//
// tagPatterns uses the same syntax as branches/git_tags in the config:
//
//	["^dev-"]              → only tags starting with "dev-"
//	["^dev-", "!^dev-keep"]→ dev- tags, excluding dev-keep*
//	[]                     → ALL tags are candidates (dangerous, use with care)
func ApplyRetention(ctx context.Context, reg Registry, repo string, tagPatterns []string, policy config.RetentionPolicy) (*RetentionResult, error) {
	if !policy.Active() {
		return nil, fmt.Errorf("retention: no active policy (all values zero)")
	}

	result := &RetentionResult{
		Provider: reg.Provider(),
		Repo:     repo,
	}

	// List all tags
	tags, err := reg.ListTags(ctx, repo)
	if err != nil {
		return result, fmt.Errorf("retention: listing tags: %w", err)
	}

	// Convert tag templates to match patterns
	patterns := templatesToPatterns(tagPatterns)

	// Filter tags that match the pattern set
	var candidates []TagInfo
	for _, t := range tags {
		if config.MatchPatterns(patterns, t.Name) {
			candidates = append(candidates, t)
		}
	}

	result.Matched = len(candidates)

	if len(candidates) == 0 {
		return result, nil
	}

	// Sort by CreatedAt descending (newest first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CreatedAt.After(candidates[j].CreatedAt)
	})

	// Apply retention policies — mark which tags to keep
	keepSet := applyPolicies(candidates, policy)

	// Count kept
	for _, keep := range keepSet {
		if keep {
			result.Kept++
		}
	}

	// Delete tags not in the keep set
	for i, tag := range candidates {
		if keepSet[i] {
			continue
		}
		if err := reg.DeleteTag(ctx, repo, tag.Name); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("deleting %s: %w", tag.Name, err))
		} else {
			result.Deleted = append(result.Deleted, tag.Name)
		}
	}

	return result, nil
}

// applyPolicies evaluates all retention rules and returns a keep/prune decision
// for each candidate. candidates must be sorted newest-first.
// Policies are additive: a tag is kept if ANY rule marks it.
func applyPolicies(candidates []TagInfo, policy config.RetentionPolicy) []bool {
	keepSet := make([]bool, len(candidates))

	// keep_last: keep the N most recent
	if policy.KeepLast > 0 {
		for i := 0; i < len(candidates) && i < policy.KeepLast; i++ {
			keepSet[i] = true
		}
	}

	// Time-bucket policies: for each bucket, keep the newest tag that falls in it.
	if policy.KeepDaily > 0 {
		applyTimeBucket(candidates, keepSet, policy.KeepDaily, truncateToDay)
	}
	if policy.KeepWeekly > 0 {
		applyTimeBucket(candidates, keepSet, policy.KeepWeekly, truncateToWeek)
	}
	if policy.KeepMonthly > 0 {
		applyTimeBucket(candidates, keepSet, policy.KeepMonthly, truncateToMonth)
	}
	if policy.KeepYearly > 0 {
		applyTimeBucket(candidates, keepSet, policy.KeepYearly, truncateToYear)
	}

	return keepSet
}

// bucketFn truncates a time to the start of its bucket period.
type bucketFn func(time.Time) time.Time

// applyTimeBucket keeps the newest tag in each of the last N distinct time buckets.
// candidates must be sorted newest-first.
func applyTimeBucket(candidates []TagInfo, keepSet []bool, count int, bucket bucketFn) {
	seen := make(map[time.Time]bool)

	for i, tag := range candidates {
		if tag.CreatedAt.IsZero() {
			continue
		}

		key := bucket(tag.CreatedAt)
		if seen[key] {
			continue // already have a newer tag for this bucket
		}

		seen[key] = true
		keepSet[i] = true

		if len(seen) >= count {
			break
		}
	}
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func truncateToWeek(t time.Time) time.Time {
	// ISO week: Monday is start of week
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday = 7
	}
	d := t.AddDate(0, 0, -(weekday - 1))
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, t.Location())
}

func truncateToMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

func truncateToYear(t time.Time) time.Time {
	return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
}

// templatesToPatterns converts StageFreight tag templates into regex patterns
// suitable for config.MatchPatterns.
//
// Template variables like {sha:8}, {version}, {branch} are replaced with
// regex wildcards (.+) so the pattern matches any resolved value.
//
// Examples:
//
//	"dev-{sha:8}"    → "^dev-.+$"
//	"{version}"      → "^.+$"
//	"latest"         → "^latest$"
//	"!{branch}-rc"   → "!^.+-rc$"
func templatesToPatterns(templates []string) []string {
	if len(templates) == 0 {
		return nil
	}

	patterns := make([]string, 0, len(templates))
	for _, tmpl := range templates {
		patterns = append(patterns, templateToPattern(tmpl))
	}
	return patterns
}

// templateToPattern converts a single tag template to a regex pattern.
func templateToPattern(tmpl string) string {
	// Preserve negation prefix
	prefix := ""
	s := tmpl
	if len(s) > 0 && s[0] == '!' {
		prefix = "!"
		s = s[1:]
	}

	// Replace all {name} and {name:param} with .+
	result := make([]byte, 0, len(s)*2)
	i := 0
	for i < len(s) {
		if s[i] == '{' {
			// Find matching close brace
			j := i + 1
			for j < len(s) && s[j] != '}' {
				j++
			}
			if j < len(s) {
				// Replace {…} with .+
				result = append(result, '.', '+')
				i = j + 1
				continue
			}
		}
		// Escape regex metacharacters in literal parts
		if isRegexMeta(s[i]) {
			result = append(result, '\\')
		}
		result = append(result, s[i])
		i++
	}

	return prefix + "^" + string(result) + "$"
}

func isRegexMeta(c byte) bool {
	switch c {
	case '.', '+', '*', '?', '(', ')', '[', ']', '{', '}', '\\', '^', '$', '|':
		return true
	}
	return false
}
