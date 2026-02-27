package dependency

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sofmeright/stagefreight/src/lint/modules/freshness"
)

// UpdateResult holds the outcome of a dependency update run.
type UpdateResult struct {
	Applied           []AppliedUpdate
	Skipped           []SkippedDep
	Verified          bool
	VerifyLog         string
	VerifyErr         error
	Artifacts         []string
	TouchedModuleDirs []string // repoRoot-relative Go module dirs that were updated
}

// AppliedUpdate records a single dependency that was successfully updated.
type AppliedUpdate struct {
	Dep        freshness.Dependency
	OldVer     string
	NewVer     string
	UpdateType string // "major", "minor", "patch", "tag"
	CVEsFixed  []string
}

// Update resolves, filters, applies, verifies, and generates artifacts for dependency updates.
func Update(ctx context.Context, cfg UpdateConfig, deps []freshness.Dependency) (*UpdateResult, error) {
	result := &UpdateResult{}

	// 1. Discover repo root
	repoRoot, err := discoverRepoRoot(cfg.RootDir)
	if err != nil {
		return result, fmt.Errorf("not a git repository: %w", err)
	}

	// 2. Check tracked files are clean
	if err := checkGitClean(ctx, repoRoot); err != nil {
		return result, err
	}

	// 3. Detect git-tracked files
	trackedFiles, err := gitTrackedFiles(ctx, repoRoot)
	if err != nil {
		return result, fmt.Errorf("listing tracked files: %w", err)
	}

	// 4. Filter update candidates
	candidates, skipped := FilterUpdateCandidates(deps, cfg, trackedFiles)
	result.Skipped = skipped

	if len(candidates) == 0 {
		return result, nil
	}

	// 5. Group by ecosystem and apply
	gomodDeps, dockerDeps := groupByEcosystem(candidates)

	if len(gomodDeps) > 0 {
		applied, goSkipped, touchedDirs, err := applyGoUpdates(ctx, gomodDeps, repoRoot)
		if err != nil {
			return result, fmt.Errorf("applying Go updates: %w", err)
		}
		result.Applied = append(result.Applied, applied...)
		result.Skipped = append(result.Skipped, goSkipped...)
		result.TouchedModuleDirs = touchedDirs
	}

	if len(dockerDeps) > 0 {
		applied, dkSkipped, err := applyDockerfileUpdates(dockerDeps, repoRoot)
		if err != nil {
			return result, fmt.Errorf("applying Dockerfile updates: %w", err)
		}
		result.Applied = append(result.Applied, applied...)
		result.Skipped = append(result.Skipped, dkSkipped...)
	}

	// 6. Verify — only run on Go module dirs that were actually updated
	if cfg.Verify && len(result.TouchedModuleDirs) > 0 {
		absDirs := make([]string, 0, len(result.TouchedModuleDirs))
		for _, d := range result.TouchedModuleDirs {
			absDirs = append(absDirs, filepath.Join(repoRoot, d))
		}
		log, verifyErr := Verify(ctx, absDirs, repoRoot, true, cfg.Vulncheck)
		result.Verified = true
		result.VerifyLog = log
		result.VerifyErr = verifyErr
	}

	// 7. Generate artifacts
	outputDir := cfg.OutputDir
	if outputDir == "" {
		outputDir = ".stagefreight/deps"
	}
	if !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(repoRoot, outputDir)
	}

	artifacts, err := GenerateArtifacts(ctx, repoRoot, outputDir, result, cfg.Bundle)
	if err != nil {
		return result, fmt.Errorf("generating artifacts: %w", err)
	}
	result.Artifacts = artifacts

	return result, nil
}

func groupByEcosystem(deps []freshness.Dependency) (gomod, docker []freshness.Dependency) {
	for _, dep := range deps {
		switch dep.Ecosystem {
		case freshness.EcosystemGoMod:
			gomod = append(gomod, dep)
		case freshness.EcosystemDockerImage, freshness.EcosystemDockerTool:
			docker = append(docker, dep)
		}
	}
	return
}

// discoverRepoRoot finds the git repository root from the given directory.
func discoverRepoRoot(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// checkGitClean verifies that tracked files have no uncommitted changes.
func checkGitClean(ctx context.Context, repoRoot string) error {
	// Check unstaged changes
	unstaged := exec.CommandContext(ctx, "git", "-C", repoRoot, "diff", "--quiet")
	if err := unstaged.Run(); err != nil {
		paths, _ := gitDirtyPaths(ctx, repoRoot, false)
		return fmt.Errorf("tracked files have unstaged changes:\n%s", strings.Join(paths, "\n"))
	}

	// Check staged changes
	staged := exec.CommandContext(ctx, "git", "-C", repoRoot, "diff", "--cached", "--quiet")
	if err := staged.Run(); err != nil {
		paths, _ := gitDirtyPaths(ctx, repoRoot, true)
		return fmt.Errorf("tracked files have staged changes:\n%s", strings.Join(paths, "\n"))
	}

	return nil
}

func gitDirtyPaths(ctx context.Context, repoRoot string, cached bool) ([]string, error) {
	args := []string{"-C", repoRoot, "diff", "--name-only"}
	if cached {
		args = append(args, "--cached")
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var paths []string
	for _, l := range lines {
		if l != "" {
			paths = append(paths, "  "+l)
		}
	}
	return paths, nil
}

// gitTrackedFiles returns a set of repo-root-relative paths for all tracked files.
func gitTrackedFiles(ctx context.Context, repoRoot string) (map[string]bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "ls-files")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	tracked := make(map[string]bool, len(lines))
	for _, l := range lines {
		if l != "" {
			tracked[l] = true
		}
	}
	return tracked, nil
}
