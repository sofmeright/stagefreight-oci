package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/sofmeright/stagefreight/src/badge"
	"github.com/sofmeright/stagefreight/src/build"
	"github.com/sofmeright/stagefreight/src/fonts"
	"github.com/sofmeright/stagefreight/src/forge"
)

var (
	rbVersion string
	rbStatus  string
	rbPath    string
	rbBranch  string
	rbLocal   bool
)

var releaseBadgeCmd = &cobra.Command{
	Use:   "badge",
	Short: "Generate and commit release badge SVG",
	Long: `Generate a release status badge SVG and commit it to the repository
via the forge API (no local clone needed).

Can also write the badge locally with --local.`,
	RunE: runReleaseBadge,
}

func init() {
	releaseBadgeCmd.Flags().StringVar(&rbVersion, "version", "", "version to display (default: detected from git)")
	releaseBadgeCmd.Flags().StringVar(&rbStatus, "status", "passed", "badge status: passed, warning, critical")
	releaseBadgeCmd.Flags().StringVar(&rbPath, "path", "", "repo path for badge file (default: from config)")
	releaseBadgeCmd.Flags().StringVar(&rbBranch, "branch", "", "branch to commit badge to (default: from config)")
	releaseBadgeCmd.Flags().BoolVar(&rbLocal, "local", false, "write badge to local filesystem instead of forge API")

	releaseCmd.AddCommand(releaseBadgeCmd)
}

func runReleaseBadge(cmd *cobra.Command, args []string) error {
	rootDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Resolve version
	version := rbVersion
	if version == "" {
		versionInfo, err := build.DetectVersion(rootDir)
		if err != nil {
			return fmt.Errorf("detecting version: %w", err)
		}
		version = versionInfo.Version
	}

	// Resolve badge path from config
	badgePath := rbPath
	if badgePath == "" {
		badgePath = cfg.Release.Badge.Path
	}

	// Generate SVG
	metrics, err := badge.LoadBuiltinFont(fonts.DefaultFont, 11)
	if err != nil {
		return fmt.Errorf("loading badge font: %w", err)
	}
	eng := badge.New(metrics)
	svg := eng.Generate(badge.Badge{
		Label: "release",
		Value: version,
		Color: badge.StatusColor(rbStatus),
	})

	// Write locally if requested
	if rbLocal {
		if err := os.MkdirAll(filepath.Dir(badgePath), 0o755); err != nil {
			return fmt.Errorf("creating badge directory: %w", err)
		}
		if err := os.WriteFile(badgePath, []byte(svg), 0o644); err != nil {
			return fmt.Errorf("writing badge: %w", err)
		}
		fmt.Printf("  badge → %s\n", badgePath)
		return nil
	}

	// Commit via forge API
	ctx := context.Background()

	remoteURL, err := detectRemoteURL(rootDir)
	if err != nil {
		return fmt.Errorf("detecting remote: %w", err)
	}

	provider := forge.DetectProvider(remoteURL)
	forgeClient, err := newForgeClient(provider, remoteURL)
	if err != nil {
		return err
	}

	branch := rbBranch
	if branch == "" {
		branch = cfg.Release.Badge.Branch
	}

	if err := forgeClient.CommitFile(ctx, forge.CommitFileOptions{
		Branch:  branch,
		Path:    badgePath,
		Content: []byte(svg),
		Message: fmt.Sprintf("chore: update release badge to %s", version),
	}); err != nil {
		return fmt.Errorf("committing badge: %w", err)
	}

	fmt.Printf("  badge %s → %s on %s\n", colorGreen("✓"), badgePath, branch)

	// Sync badge to other forges
	if len(cfg.Release.Sync) > 0 {
		currentTag := os.Getenv("CI_COMMIT_TAG")
		currentBranch := resolveBranchFromEnv()
		for _, target := range cfg.Release.Sync {
			if !target.SyncBadge {
				continue
			}
			if !syncAllowed(target, currentTag, currentBranch) {
				continue
			}

			syncClient, err := newSyncForgeClient(target)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  warning: badge sync to %s: %v\n", target.Name, err)
				continue
			}

			if err := syncClient.CommitFile(ctx, forge.CommitFileOptions{
				Branch:  branch,
				Path:    badgePath,
				Content: []byte(svg),
				Message: fmt.Sprintf("chore: update release badge to %s", version),
			}); err != nil {
				fmt.Fprintf(os.Stderr, "  warning: badge sync to %s: %v\n", target.Name, err)
				continue
			}
			fmt.Printf("  → badge synced to %s\n", target.Name)
		}
	}

	return nil
}

