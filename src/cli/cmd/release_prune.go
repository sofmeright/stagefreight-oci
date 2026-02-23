package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/sofmeright/stagefreight/src/config"
	"github.com/sofmeright/stagefreight/src/forge"
	"github.com/sofmeright/stagefreight/src/retention"
)

var rpDryRun bool

var releasePruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Prune old releases using retention policy",
	Long: `Delete old releases on the detected forge using the retention
policy from .stagefreight.yml.

Tag templates from release.tags are converted to patterns so
only releases matching the configured tag scheme are candidates.

Use --dry-run to preview what would be deleted without deleting.`,
	RunE: runReleasePrune,
}

func init() {
	releasePruneCmd.Flags().BoolVar(&rpDryRun, "dry-run", false, "show what would be deleted without deleting")

	releaseCmd.AddCommand(releasePruneCmd)
}

func runReleasePrune(cmd *cobra.Command, args []string) error {
	if !cfg.Release.Retention.Active() {
		return fmt.Errorf("no retention policy configured in release.retention")
	}

	rootDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	ctx := context.Background()

	// Detect forge
	remoteURL, err := detectRemoteURL(rootDir)
	if err != nil {
		return fmt.Errorf("detecting remote: %w", err)
	}

	provider := forge.DetectProvider(remoteURL)
	if provider == forge.Unknown {
		return fmt.Errorf("could not detect forge from remote URL: %s", remoteURL)
	}

	forgeClient, err := newForgeClient(provider, remoteURL)
	if err != nil {
		return err
	}

	// Resolve tag patterns from templates
	var patterns []string
	if len(cfg.Release.Tags) > 0 {
		patterns = retention.TemplatesToPatterns(cfg.Release.Tags)
	}

	if rpDryRun {
		return runReleasePruneDryRun(ctx, forgeClient, patterns)
	}

	store := &forgeStore{forge: forgeClient}
	result, err := retention.Apply(ctx, store, patterns, cfg.Release.Retention)
	if err != nil {
		return fmt.Errorf("retention: %w", err)
	}

	fmt.Printf("  release retention: matched=%d kept=%d deleted=%d\n",
		result.Matched, result.Kept, len(result.Deleted))

	for _, d := range result.Deleted {
		fmt.Printf("    - %s\n", d)
	}
	for _, e := range result.Errors {
		fmt.Fprintf(os.Stderr, "    error: %v\n", e)
	}

	return nil
}

func runReleasePruneDryRun(ctx context.Context, f forge.Forge, patterns []string) error {
	store := &forgeStore{forge: f}
	items, err := store.List(ctx)
	if err != nil {
		return fmt.Errorf("listing releases: %w", err)
	}

	// Filter by patterns
	var candidates []retention.Item
	for _, item := range items {
		if len(patterns) == 0 || matchesPatterns(patterns, item.Name) {
			candidates = append(candidates, item)
		}
	}

	if len(candidates) == 0 {
		fmt.Println("  no releases match the configured tag patterns")
		return nil
	}

	// Sort newest first
	sortItems(candidates)

	// Apply policies (dry run — just compute keep set)
	keepSet := retention.ApplyPolicies(candidates, cfg.Release.Retention)

	var keepCount, pruneCount int
	for i, item := range candidates {
		if keepSet[i] {
			keepCount++
			if verbose {
				fmt.Printf("    keep  %s\n", item.Name)
			}
		} else {
			pruneCount++
			fmt.Printf("    prune %s\n", item.Name)
		}
	}

	fmt.Printf("  dry run: matched=%d keep=%d prune=%d\n",
		len(candidates), keepCount, pruneCount)

	return nil
}

// forgeStore adapts forge.Forge to the retention.Store interface.
type forgeStore struct {
	forge forge.Forge
}

func (s *forgeStore) List(ctx context.Context) ([]retention.Item, error) {
	releases, err := s.forge.ListReleases(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]retention.Item, len(releases))
	for i, r := range releases {
		items[i] = retention.Item{
			Name:      r.TagName,
			CreatedAt: r.CreatedAt,
		}
	}
	return items, nil
}

func (s *forgeStore) Delete(ctx context.Context, name string) error {
	return s.forge.DeleteRelease(ctx, name)
}

// matchesPatterns is a thin wrapper for dry-run pattern matching.
func matchesPatterns(patterns []string, value string) bool {
	return config.MatchPatterns(patterns, value)
}

// sortItems sorts items newest-first.
func sortItems(items []retention.Item) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].CreatedAt.After(items[j-1].CreatedAt); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
