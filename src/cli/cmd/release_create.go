package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/sofmeright/stagefreight/src/build"
	"github.com/sofmeright/stagefreight/src/config"
	"github.com/sofmeright/stagefreight/src/forge"
	"github.com/sofmeright/stagefreight/src/gitver"
	"github.com/sofmeright/stagefreight/src/output"
	"github.com/sofmeright/stagefreight/src/registry"
	"github.com/sofmeright/stagefreight/src/release"
	"github.com/sofmeright/stagefreight/src/retention"
)

var (
	rcTag             string
	rcName            string
	rcNotesFile       string
	rcSecuritySummary string
	rcDraft           bool
	rcPrerelease      bool
	rcAssets          []string
	rcRegistryLinks   bool
	rcCatalogLinks    bool
	rcSkipSync        bool
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
	releaseCreateCmd.Flags().StringVar(&rcSecuritySummary, "security-summary", "", "path to security output directory (reads summary.md)")
	releaseCreateCmd.Flags().BoolVar(&rcDraft, "draft", false, "create as draft release")
	releaseCreateCmd.Flags().BoolVar(&rcPrerelease, "prerelease", false, "mark as prerelease")
	releaseCreateCmd.Flags().StringSliceVar(&rcAssets, "asset", nil, "files to attach to release (repeatable)")
	releaseCreateCmd.Flags().BoolVar(&rcRegistryLinks, "registry-links", true, "add registry image links to release")
	releaseCreateCmd.Flags().BoolVar(&rcCatalogLinks, "catalog-links", true, "add GitLab Catalog link to release")
	releaseCreateCmd.Flags().BoolVar(&rcSkipSync, "skip-sync", false, "skip syncing to other forges")

	releaseCmd.AddCommand(releaseCreateCmd)
}

// actionResult tracks the outcome of a single release action.
type actionResult struct {
	Name string
	OK   bool
	Err  error
}

// releaseReport collects all release action outcomes for rendering.
type releaseReport struct {
	Tag, Forge, URL string
	Assets          []actionResult
	Links           []actionResult
	Tags            []actionResult
}

func runReleaseCreate(cmd *cobra.Command, args []string) error {
	rootDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	ctx := context.Background()
	color := output.UseColor()
	w := os.Stdout

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

	// Load security summary if provided
	var secTile, secBody string
	if rcSecuritySummary != "" {
		summaryPath := rcSecuritySummary + "/summary.md"
		data, err := os.ReadFile(summaryPath)
		if err != nil {
			// Not fatal — security scan may have been skipped
			if verbose {
				fmt.Fprintf(os.Stderr, "note: no security summary at %s: %v\n", summaryPath, err)
			}
		} else {
			content := strings.TrimSpace(string(data))
			if content != "" {
				parts := strings.SplitN(content, "\n", 2)
				secTile = strings.TrimSpace(parts[0])
				secBody = content
			}
		}
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
		sha := versionInfo.SHA
		if len(sha) > 8 {
			sha = sha[:8]
		}
		input := release.NotesInput{
			RepoDir:      rootDir,
			ToRef:        tag,
			SecurityTile: secTile,
			SecurityBody: secBody,
			Version:      versionInfo.Version,
			SHA:          sha,
			IsPrerelease: versionInfo.IsPrerelease,
		}
		notes, err = release.GenerateNotes(input)
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

	// ── Collect all results ──
	start := time.Now()
	report := releaseReport{
		Tag:   tag,
		Forge: string(provider),
	}

	// Create release
	rel, createErr := forgeClient.CreateRelease(ctx, forge.ReleaseOptions{
		TagName:     tag,
		Name:        name,
		Description: notes,
		Draft:       rcDraft,
		Prerelease:  rcPrerelease,
	})
	if createErr != nil {
		return fmt.Errorf("creating release: %w", createErr)
	}
	report.URL = rel.URL

	// Upload assets
	for _, assetPath := range rcAssets {
		assetName := assetPath
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
			report.Assets = append(report.Assets, actionResult{Name: assetName, Err: err})
			fmt.Fprintf(os.Stderr, "warning: failed to upload %s: %v\n", assetPath, err)
		} else {
			report.Assets = append(report.Assets, actionResult{Name: assetName, OK: true})
		}
	}

	// Add registry image links (deduplicate by URL)
	if rcRegistryLinks && len(cfg.Docker.Registries) > 0 {
		linkedURLs := make(map[string]bool)
		for _, reg := range cfg.Docker.Registries {
			regProvider := reg.Provider
			if regProvider == "" {
				regProvider = build.DetectProvider(reg.URL)
			}
			if p, err := registry.CanonicalProvider(regProvider); err == nil {
				regProvider = p
			} else {
				regProvider = "generic"
			}

			link := buildRegistryLink(reg, tag, regProvider)
			if linkedURLs[link.URL] {
				continue
			}
			linkedURLs[link.URL] = true

			if err := forgeClient.AddReleaseLink(ctx, rel.ID, link); err != nil {
				report.Links = append(report.Links, actionResult{Name: link.Name, Err: err})
				fmt.Fprintf(os.Stderr, "warning: failed to add registry link for %s: %v\n", reg.URL, err)
			} else {
				report.Links = append(report.Links, actionResult{Name: link.Name, OK: true})
			}
		}
	}

	// Add GitLab Catalog link
	if rcCatalogLinks && cfg.GitlabComponent.Catalog && provider == forge.GitLab {
		catalogLink := buildCatalogLink(remoteURL, tag)
		if catalogLink.URL != "" {
			if err := forgeClient.AddReleaseLink(ctx, rel.ID, catalogLink); err != nil {
				report.Links = append(report.Links, actionResult{Name: catalogLink.Name, Err: err})
				fmt.Fprintf(os.Stderr, "warning: failed to add catalog link: %v\n", err)
			} else {
				report.Links = append(report.Links, actionResult{Name: catalogLink.Name, OK: true})
			}
		}
	}

	// Auto-tagging: create rolling releases for configured tag templates
	if len(cfg.Release.Tags) > 0 {
		currentTag := os.Getenv("CI_COMMIT_TAG")
		if config.MatchPatternsWithPolicy(cfg.Release.GitTags, currentTag, cfg.Git.Policy.Tags) || currentTag == "" {
			rollingTags := gitver.ResolveTags(cfg.Release.Tags, versionInfo)
			for _, rt := range rollingTags {
				if rt == tag || rt == "" {
					continue
				}
				_, err := forgeClient.CreateRelease(ctx, forge.ReleaseOptions{
					TagName:     rt,
					Name:        rt,
					Description: fmt.Sprintf("Rolling tag for %s", tag),
					Prerelease:  rcPrerelease,
				})
				if err != nil {
					// Rolling tag may already exist — try delete then recreate
					_ = forgeClient.DeleteRelease(ctx, rt)
					_, err = forgeClient.CreateRelease(ctx, forge.ReleaseOptions{
						TagName:     rt,
						Name:        rt,
						Description: fmt.Sprintf("Rolling tag for %s", tag),
						Prerelease:  rcPrerelease,
					})
					if err != nil {
						report.Tags = append(report.Tags, actionResult{Name: rt, Err: err})
						fmt.Fprintf(os.Stderr, "warning: rolling tag %s: %v\n", rt, err)
						continue
					}
				}
				report.Tags = append(report.Tags, actionResult{Name: rt, OK: true})
			}
		}
	}

	elapsed := time.Since(start)

	// ── Release section ──
	overallStatus := "created"
	overallIcon := "success"
	if hasActionFailures(report.Assets) || hasActionFailures(report.Links) || hasActionFailures(report.Tags) {
		overallStatus = "partial"
		overallIcon = "skipped" // yellow icon
	}

	output.SectionStart(w, "sf_release", "Release")
	sec := output.NewSection(w, "Release", elapsed, color)
	sec.Row("%s  →  %s   %s  %s", tag, provider, output.StatusIcon(overallIcon, color), overallStatus)
	sec.Row("%s", report.URL)

	if len(report.Assets) > 0 || len(report.Links) > 0 || len(report.Tags) > 0 {
		sec.Row("")
		if len(report.Assets) > 0 {
			renderCheckpoint(sec, color, "assets", report.Assets)
		}
		if len(report.Links) > 0 {
			renderCheckpoint(sec, color, "links", report.Links)
		}
		if len(report.Tags) > 0 {
			renderCheckpoint(sec, color, "tags", report.Tags)
		}
	}

	sec.Close()
	output.SectionEnd(w, "sf_release")

	// ── Sync section ──
	if !rcSkipSync && len(cfg.Release.Sync) > 0 {
		currentTag := os.Getenv("CI_COMMIT_TAG")
		currentBranch := resolveBranchFromEnv()

		var syncResults []actionResult
		syncStart := time.Now()

		for _, target := range cfg.Release.Sync {
			if !syncAllowed(target, currentTag, currentBranch, cfg.Git.Policy) {
				if verbose {
					fmt.Fprintf(os.Stderr, "skip sync: %s (tag=%q branch=%q not allowed)\n", target.Name, currentTag, currentBranch)
				}
				continue
			}

			syncClient, err := newSyncForgeClient(target)
			if err != nil {
				syncResults = append(syncResults, actionResult{Name: target.Name, Err: err})
				fmt.Fprintf(os.Stderr, "warning: sync to %s: %v\n", target.Name, err)
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
					syncResults = append(syncResults, actionResult{Name: target.Name, Err: err})
					fmt.Fprintf(os.Stderr, "warning: sync release to %s: %v\n", target.Name, err)
					continue
				}

				syncResults = append(syncResults, actionResult{Name: fmt.Sprintf("%s: %s", target.Name, syncRel.URL), OK: true})

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
							fmt.Fprintf(os.Stderr, "warning: sync asset %s to %s: %v\n", assetName, target.Name, err)
						}
					}
				}
			}
		}

		if len(syncResults) > 0 {
			syncElapsed := time.Since(syncStart)
			output.SectionStart(w, "sf_sync", "Sync")
			syncSec := output.NewSection(w, "Sync", syncElapsed, color)
			for _, r := range syncResults {
				if r.OK {
					syncSec.Row("%s %s", output.StatusIcon("success", color), r.Name)
				} else {
					msg := "unknown error"
					if r.Err != nil {
						msg = r.Err.Error()
					}
					syncSec.Row("%s %s: %s", output.StatusIcon("failed", color), r.Name, msg)
				}
			}
			syncSec.Close()
			output.SectionEnd(w, "sf_sync")
		}
	}

	// ── Retention section ──
	if cfg.Release.Retention.Active() {
		retStart := time.Now()
		var patterns []string
		if len(cfg.Release.Tags) > 0 {
			patterns = retention.TemplatesToPatterns(cfg.Release.Tags)
		}
		store := &forgeStore{forge: forgeClient}
		result, retErr := retention.Apply(ctx, store, patterns, cfg.Release.Retention)

		retElapsed := time.Since(retStart)

		output.SectionStart(w, "sf_retention", "Retention")
		retSec := output.NewSection(w, "Retention", retElapsed, color)

		if retErr != nil {
			retSec.Row("error: %v", retErr)
			fmt.Fprintf(os.Stderr, "warning: release retention: %v\n", retErr)
		} else {
			retSec.Row("%-16s%d", "matched", result.Matched)
			retSec.Row("%-16s%d", "kept", result.Kept)
			retSec.Row("%-16s%d", "pruned", len(result.Deleted))
			for _, d := range result.Deleted {
				retSec.Row("  - %s", d)
			}
		}

		retSec.Close()
		output.SectionEnd(w, "sf_retention")
	}

	return nil
}

// renderCheckpoint renders a checkpoint line with pass/fail count, expanding failures.
func renderCheckpoint(sec *output.Section, color bool, label string, results []actionResult) {
	total := len(results)
	ok := 0
	var failed []actionResult
	for _, r := range results {
		if r.OK {
			ok++
		} else {
			failed = append(failed, r)
		}
	}

	status := "success"
	if ok != total {
		status = "failed"
	}
	icon := output.StatusIcon(status, color)

	sec.Row("%s %-7s %d/%d", icon, label+":", ok, total)

	for _, r := range failed {
		msg := "unknown error"
		if r.Err != nil {
			msg = r.Err.Error()
		}
		sec.Row("  - %s: %s", r.Name, msg)
	}
}

// hasActionFailures returns true if any result has a failure.
func hasActionFailures(results []actionResult) bool {
	for _, r := range results {
		if !r.OK {
			return true
		}
	}
	return false
}

// buildRegistryLink creates a forge release link for a registry image.
// Constructs vendor-aware URLs (e.g., Docker Hub web URL vs generic registry).
func buildRegistryLink(reg config.RegistryConfig, tag string, provider string) forge.ReleaseLink {
	imageRef := fmt.Sprintf("%s/%s:%s", reg.URL, reg.Path, tag)

	var webURL string
	switch provider {
	case "docker":
		// Docker Hub web URL: hub.docker.com/r/org/repo/tags
		webURL = fmt.Sprintf("https://hub.docker.com/r/%s/tags?name=%s", reg.Path, tag)
	case "github":
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
	case "docker":
		return "Docker Hub"
	case "github":
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

// buildCatalogLink creates a GitLab Catalog release link for a component project.
func buildCatalogLink(remoteURL, tag string) forge.ReleaseLink {
	// Try CI env first (most reliable in GitLab CI).
	if serverURL := os.Getenv("CI_SERVER_URL"); serverURL != "" {
		if projectPath := os.Getenv("CI_PROJECT_PATH"); projectPath != "" {
			return forge.ReleaseLink{
				Name:     fmt.Sprintf("GitLab Catalog %s", tag),
				URL:      fmt.Sprintf("%s/explore/catalog/%s", serverURL, projectPath),
				LinkType: "other",
			}
		}
	}

	// Fallback: extract from remote URL.
	projectPath := projectPathFromRemote(remoteURL)
	if projectPath == "" {
		return forge.ReleaseLink{}
	}

	baseURL := forge.BaseURL(remoteURL)
	return forge.ReleaseLink{
		Name:     fmt.Sprintf("GitLab Catalog %s", tag),
		URL:      fmt.Sprintf("%s/explore/catalog/%s", baseURL, projectPath),
		LinkType: "other",
	}
}

// projectPathFromRemote extracts the "org/repo" project path from a git remote URL.
// Handles SSH (git@host:org/repo.git) and HTTPS (https://host/org/repo.git).
func projectPathFromRemote(remoteURL string) string {
	url := remoteURL

	// SSH format: git@host:org/repo.git or git@host:port:org/repo.git
	if idx := strings.Index(url, ":"); idx >= 0 && !strings.HasPrefix(url, "http") {
		path := url[idx+1:]
		// Handle SSH with port: git@host:port/org/repo.git
		if slashIdx := strings.Index(path, "/"); slashIdx >= 0 {
			// Check if part before / is a port number
			possiblePort := path[:slashIdx]
			isPort := true
			for _, c := range possiblePort {
				if c < '0' || c > '9' {
					isPort = false
					break
				}
			}
			if isPort {
				path = path[slashIdx+1:]
			}
		}
		return strings.TrimSuffix(path, ".git")
	}

	// HTTPS format: https://host/org/repo.git
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(url, prefix) {
			withoutScheme := strings.TrimPrefix(url, prefix)
			// Remove host
			if slashIdx := strings.Index(withoutScheme, "/"); slashIdx >= 0 {
				path := withoutScheme[slashIdx+1:]
				return strings.TrimSuffix(path, ".git")
			}
		}
	}

	return ""
}

// syncAllowed checks if a sync target should be activated for the current tag/branch.
// Uses policy-aware pattern matching — supports regex, ! negation, and policy name resolution.
func syncAllowed(target config.SyncTarget, tag, branch string, policy config.GitPolicyConfig) bool {
	if !config.MatchPatternsWithPolicy(target.Branches, branch, policy.Branches) {
		return false
	}
	if tag != "" && !config.MatchPatternsWithPolicy(target.Tags, tag, policy.Tags) {
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
		return forge.NewGitHub(baseURL), nil
	case forge.Gitea:
		return forge.NewGitea(baseURL), nil
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
		gh := forge.NewGitHub(target.URL)
		if target.Credentials != "" {
			token := os.Getenv(target.Credentials + "_TOKEN")
			if token == "" {
				return nil, fmt.Errorf("sync target %s: %s_TOKEN env var not set", target.Name, target.Credentials)
			}
			gh.Token = token
		}
		if target.ProjectID != "" {
			gh.Owner = ownerFromPath(target.ProjectID)
			gh.Repo = repoFromPath(target.ProjectID)
		}
		return gh, nil
	case "gitea":
		gt := forge.NewGitea(target.URL)
		if target.Credentials != "" {
			token := os.Getenv(target.Credentials + "_TOKEN")
			if token == "" {
				return nil, fmt.Errorf("sync target %s: %s_TOKEN env var not set", target.Name, target.Credentials)
			}
			gt.Token = token
		}
		if target.ProjectID != "" {
			gt.Owner = ownerFromPath(target.ProjectID)
			gt.Repo = repoFromPath(target.ProjectID)
		}
		return gt, nil
	default:
		return nil, fmt.Errorf("unknown sync provider: %s", target.Provider)
	}
}
