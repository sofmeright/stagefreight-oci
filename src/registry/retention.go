package registry

import (
	"context"
	"fmt"

	"github.com/sofmeright/stagefreight/src/config"
	"github.com/sofmeright/stagefreight/src/retention"
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
	store := &registryStore{reg: reg, repo: repo}
	patterns := retention.TemplatesToPatterns(tagPatterns)

	result, err := retention.Apply(ctx, store, patterns, policy)
	if err != nil {
		return &RetentionResult{
			Provider: reg.Provider(),
			Repo:     repo,
		}, err
	}

	return &RetentionResult{
		Provider: reg.Provider(),
		Repo:     repo,
		Matched:  result.Matched,
		Kept:     result.Kept,
		Deleted:  result.Deleted,
		Errors:   result.Errors,
	}, nil
}

// registryStore adapts the Registry interface to the retention.Store interface.
type registryStore struct {
	reg  Registry
	repo string
}

func (s *registryStore) List(ctx context.Context) ([]retention.Item, error) {
	tags, err := s.reg.ListTags(ctx, s.repo)
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}

	items := make([]retention.Item, len(tags))
	for i, t := range tags {
		items[i] = retention.Item{
			Name:      t.Name,
			CreatedAt: t.CreatedAt,
		}
	}
	return items, nil
}

func (s *registryStore) Delete(ctx context.Context, name string) error {
	return s.reg.DeleteTag(ctx, s.repo, name)
}
