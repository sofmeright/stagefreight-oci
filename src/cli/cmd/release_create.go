package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/sofmeright/stagefreight/src/build"
	"github.com/sofmeright/stagefreight/src/config"
	"github.com/sofmeright/stagefreight/src/forge"
	"github.com/sofmeright/stagefreight/src/release"
)

var (
	rcTag         string
	rcName        string
	rcNotesFile   string
	rcDraft       bool
	rcPrerelease  bool
	rcAssets      []string
	rcRegistryLinks bool
	rcSkipSync    bool
)

var releaseCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a release on the forge and sync to targets",
	Long: `Create a release on the detected forge (GitLab, GitHub, Gitea)
with generated or provided release notes.

Optionally uploads assets (scan artifacts, SBOMs) and adds
registry image links. Syncs to configured sync targets unless
--skip-sync is set.`,
	RunE: runReleaseCreate,
}

func init() {
	releaseCreateCmd.Flags().StringVar(&rcTag, "tag", "", "release tag (default: detected from git)")
	releaseCreateCmd.Flags().StringVar(&rcName, "name", "", "release name (default: tag)")
	releaseCreateCmd.Flags().StringVar(&rcNotesFile, "notes", "", "path to release notes markdown file")
	releaseCreateCmd.Flags().BoolVar(&rcDraft, "draft", false, "create as draft release")
	releaseCreateCmd.Flags().BoolVar(&rcPrerelease, "prerelease", false, "mark as prerelease")
	releaseCreateCmd.Flags().StringSliceVar(&rcAssets, "asset", nil, "files to attach to release (repeatable)")
	releaseCreateCmd.Flags().BoolVar(&rcRegistryLinks, "registry-links", true, "add registry image links to release")
	releaseCreateCmd.Flags().BoolVar(&rcSkipSync, "skip-sync", false, "skip syncing to other forges")

	releaseCmd.AddCommand(releaseCreateCmd)
}

func runReleaseCreate(cmd *cobra.Command, args []string) error {
	rootDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	ctx := context.Background()

	// Detect version for tag
	versionInfo, err := build.DetectVersion(rootDir)
	if err != nil {
		return fmt.Errorf("detecting version: %w", err)
	}

	tag := rcTag
	if tag == "" {
		tag = "v" + versionInfo.Version
	}
	name := rcName
	if name == "" {
		name = tag
	}

	// Generate or load release notes
	var notes string
	if rcNotesFile != "" {
		data, err := os.ReadFile(rcNotesFile)
		if err != nil {
			return fmt.Errorf("reading notes file: %w", err)
		}
		notes = string(data)
	} else {
		notes, err = release.GenerateNotes(rootDir, "", tag, "")
		if err != nil {
			return fmt.Errorf("generating notes: %w", err)
		}
	}

	// Detect forge from git remote
	remoteURL, err := detectRemoteURL(rootDir)
	if err != nil {
		return fmt.Errorf("detecting remote: %w", err)
	}

	provider := forge.DetectProvider(remoteURL)
	if provider == forge.Unknown {
		return fmt.Errorf("could not detect forge from remote URL: %s", remoteURL)
	}

	// Create forge client
	forgeClient, err := newForgeClient(provider, remoteURL)
	if err != nil {
		return err
	}

	fmt.Printf("  creating release %s on %s...\n", tag, provider)

	// Create release
	rel, err := forgeClient.CreateRelease(ctx, forge.ReleaseOptions{
		TagName:     tag,
		Name:        name,
		Description: notes,
		Draft:       rcDraft,
		Prerelease:  rcPrerelease,
	})
	if err != nil {
		return fmt.Errorf("creating release: %w", err)
	}

	fmt.Printf("  release %s → %s\n", colorGreen("✓"), rel.URL)

	// Upload assets
	for _, assetPath := range rcAssets {
		assetName := assetPath
		// Use basename for display name
		for i := len(assetPath) - 1; i >= 0; i-- {
			if assetPath[i] == '/' {
				assetName = assetPath[i+1:]
				break
			}
		}

		if err := forgeClient.UploadAsset(ctx, rel.ID, forge.Asset{
			Name:     assetName,
			FilePath: assetPath,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: failed to upload %s: %v\n", assetPath, err)
			continue
		}
		fmt.Printf("  → uploaded %s\n", assetName)
	}

	// Add registry image links
	if rcRegistryLinks && len(cfg.Docker.Registries) > 0 {
		for _, reg := range cfg.Docker.Registries {
			provider := reg.Provider
			if provider == "" {
				provider = build.DetectProvider(reg.URL)
			}

			link := buildRegistryLink(reg, tag, provider)
			if err := forgeClient.AddReleaseLink(ctx, rel.ID, link); err != nil {
				fmt.Fprintf(os.Stderr, "  warning: failed to add registry link for %s: %v\n", reg.URL, err)
				continue
			}
			fmt.Printf("  → linked %s\n", link.Name)
		}
	}

	// Sync to other forges
	if !rcSkipSync && len(cfg.Release.Sync) > 0 {
		currentTag := os.Getenv("CI_COMMIT_TAG")
		currentBranch := resolveBranchFromEnv()
		for _, target := range cfg.Release.Sync {
			if !syncAllowed(target, currentTag, currentBranch) {
				if verbose {
					fmt.Fprintf(os.Stderr, "  skip sync: %s (tag=%q branch=%q not allowed)\n", target.Name, currentTag, currentBranch)
				}
				continue
			}

			syncClient, err := newSyncForgeClient(target)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  warning: sync to %s: %v\n", target.Name, err)
				continue
			}

			if target.SyncRelease {
				syncRel, err := syncClient.CreateRelease(ctx, forge.ReleaseOptions{
					TagName:     tag,
					Name:        name,
					Description: notes,
					Draft:       rcDraft,
					Prerelease:  rcPrerelease,
				})
				if err != nil {
					fmt.Fprintf(os.Stderr, "  warning: sync release to %s: %v\n", target.Name, err)
				} else {
					fmt.Printf("  → synced release to %s: %s\n", target.Name, syncRel.URL)
				}

				// Sync assets to this target
				if target.SyncAssets {
					for _, assetPath := range rcAssets {
						assetName := assetPath
						for i := len(assetPath) - 1; i >= 0; i-- {
							if assetPath[i] == '/' {
								assetName = assetPath[i+1:]
								break
							}
						}
						if err := syncClient.UploadAsset(ctx, syncRel.ID, forge.Asset{
							Name:     assetName,
							FilePath: assetPath,
						}); err != nil {
							fmt.Fprintf(os.Stderr, "  warning: sync asset %s to %s: %v\n", assetName, target.Name, err)
						}
					}
				}
			}
		}
	}

	return nil
}

// buildRegistryLink creates a forge release link for a registry image.
// Constructs vendor-aware URLs (e.g., Docker Hub web URL vs generic registry).
func buildRegistryLink(reg config.RegistryConfig, tag string, provider string) forge.ReleaseLink {
	imageRef := fmt.Sprintf("%s/%s:%s", reg.URL, reg.Path, tag)

	var webURL string
	switch provider {
	case "dockerhub":
		// Docker Hub web URL: hub.docker.com/r/org/repo/tags
		webURL = fmt.Sprintf("https://hub.docker.com/r/%s/tags?name=%s", reg.Path, tag)
	case "ghcr":
		// GitHub Container Registry: ghcr.io/org/repo
		webURL = fmt.Sprintf("https://github.com/%s/pkgs/container/%s", ownerFromPath(reg.Path), repoFromPath(reg.Path))
	case "quay":
		webURL = fmt.Sprintf("https://quay.io/repository/%s?tag=%s", reg.Path, tag)
	case "gitlab":
		webURL = fmt.Sprintf("%s/%s/container_registry", reg.URL, reg.Path)
	case "jfrog":
		webURL = fmt.Sprintf("https://%s/ui/repos/tree/General/%s", reg.URL, reg.Path)
	default:
		webURL = imageRef
	}

	return forge.ReleaseLink{
		Name:     fmt.Sprintf("%s %s", vendorDisplayName(provider), tag),
		URL:      webURL,
		LinkType: "image",
	}
}

// vendorDisplayName returns a human-friendly name for a registry provider.
func vendorDisplayName(provider string) string {
	switch provider {
	case "dockerhub":
		return "Docker Hub"
	case "ghcr":
		return "GitHub Container Registry"
	case "quay":
		return "Quay.io"
	case "gitlab":
		return "GitLab Registry"
	case "jfrog":
		return "JFrog Artifactory"
	case "harbor":
		return "Harbor"
	default:
		return "Container Image"
	}
}

// ownerFromPath extracts the owner/org from "owner/repo" or "owner/repo/sub".
func ownerFromPath(path string) string {
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return path
}

// repoFromPath extracts the repo name from "owner/repo".
func repoFromPath(path string) string {
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			rest := path[i+1:]
			// Strip any further path components
			for j := 0; j < len(rest); j++ {
				if rest[j] == '/' {
					return rest[:j]
				}
			}
			return rest
		}
	}
	return path
}

// syncAllowed checks if a sync target should be activated for the current tag/branch.
// Uses the standard MatchPatterns from config — supports regex and ! negation.
func syncAllowed(target config.SyncTarget, tag, branch string) bool {
	if !config.MatchPatterns(target.Branches, branch) {
		return false
	}
	if tag != "" && !config.MatchPatterns(target.Tags, tag) {
		return false
	}
	return true
}

// resolveBranchFromEnv resolves the current branch from CI environment variables.
func resolveBranchFromEnv() string {
	if b := os.Getenv("CI_COMMIT_BRANCH"); b != "" {
		return b
	}
	if b := os.Getenv("GITHUB_REF_NAME"); b != "" {
		return b
	}
	return ""
}

// detectRemoteURL gets the git remote origin URL.
func detectRemoteURL(rootDir string) (string, error) {
	det, err := build.DetectRepo(rootDir)
	if err != nil {
		return "", err
	}
	if det.GitInfo != nil && det.GitInfo.Remote != "" {
		return det.GitInfo.Remote, nil
	}
	return "", fmt.Errorf("no git remote URL found")
}

// newForgeClient creates a forge client from the detected provider and remote URL.
func newForgeClient(provider forge.Provider, remoteURL string) (forge.Forge, error) {
	baseURL := forge.BaseURL(remoteURL)

	switch provider {
	case forge.GitLab:
		return forge.NewGitLab(baseURL), nil
	case forge.GitHub:
		return nil, fmt.Errorf("GitHub forge not yet implemented")
	case forge.Gitea:
		return nil, fmt.Errorf("Gitea forge not yet implemented")
	default:
		return nil, fmt.Errorf("unknown forge provider: %s", provider)
	}
}

// newSyncForgeClient creates a forge client for a sync target.
func newSyncForgeClient(target config.SyncTarget) (forge.Forge, error) {
	switch target.Provider {
	case "gitlab":
		gl := forge.NewGitLab(target.URL)
		// Override with sync-specific credentials
		if target.Credentials != "" {
			token := os.Getenv(target.Credentials + "_TOKEN")
			if token == "" {
				return nil, fmt.Errorf("sync target %s: %s_TOKEN env var not set", target.Name, target.Credentials)
			}
			gl.Token = token
		}
		if target.ProjectID != "" {
			gl.ProjectID = target.ProjectID
		}
		return gl, nil
	case "github":
		return nil, fmt.Errorf("GitHub forge not yet implemented")
	case "gitea":
		return nil, fmt.Errorf("Gitea forge not yet implemented")
	default:
		return nil, fmt.Errorf("unknown sync provider: %s", target.Provider)
	}
}
