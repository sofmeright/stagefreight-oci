package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/sofmeright/stagefreight/src/badge"
	"github.com/sofmeright/stagefreight/src/build"
	"github.com/sofmeright/stagefreight/src/build/engines"
	"github.com/sofmeright/stagefreight/src/config"
	"github.com/sofmeright/stagefreight/src/gitver"
	"github.com/sofmeright/stagefreight/src/lint"
	"github.com/sofmeright/stagefreight/src/lint/modules"
	"github.com/sofmeright/stagefreight/src/output"
	"github.com/sofmeright/stagefreight/src/registry"
	"github.com/sofmeright/stagefreight/src/version"
)

var (
	dbLocal     bool
	dbPlatforms []string
	dbTags      []string
	dbTarget    string
	dbBuildID   string
	dbSkipLint  bool
	dbDryRun    bool
)

var dockerBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build and push container images",
	Long: `Build container images using docker buildx.

Detects Dockerfiles, resolves tags from git, and pushes to configured registries.
Runs lint as a pre-build gate unless --skip-lint is set.`,
	RunE: runDockerBuild,
}

func init() {
	dockerBuildCmd.Flags().BoolVar(&dbLocal, "local", false, "build for current platform, load into daemon")
	dockerBuildCmd.Flags().StringSliceVar(&dbPlatforms, "platform", nil, "override platforms (comma-separated)")
	dockerBuildCmd.Flags().StringSliceVar(&dbTags, "tag", nil, "override/add tags")
	dockerBuildCmd.Flags().StringVar(&dbTarget, "target", "", "override Dockerfile target stage")
	dockerBuildCmd.Flags().StringVar(&dbBuildID, "build", "", "build a specific entry by ID (default: all)")
	dockerBuildCmd.Flags().BoolVar(&dbSkipLint, "skip-lint", false, "skip pre-build lint")
	dockerBuildCmd.Flags().BoolVar(&dbDryRun, "dry-run", false, "show the plan without executing")

	dockerCmd.AddCommand(dockerBuildCmd)
}

func runDockerBuild(cmd *cobra.Command, args []string) error {
	rootDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	if len(args) > 0 {
		rootDir = args[0]
	}

	ctx := context.Background()
	ci := output.IsCI()
	color := output.UseColor()
	w := os.Stdout
	pipelineStart := time.Now()

	// Inject project description from docker-readme targets for {project.description} templates
	if desc := firstDockerReadmeDescription(cfg); desc != "" {
		gitver.SetProjectDescription(desc)
	}

	// Banner — StageFreight's own identity from build-time ldflags
	output.Banner(w, output.NewBannerInfo(version.Version, version.Commit, ""), color)

	// Pipeline context block
	output.ContextBlock(w, buildContextKV())

	// --- Pre-build lint gate ---
	var lintSummary string
	if !dbSkipLint {
		output.SectionStart(w, "sf_lint", "Lint")
		var lintErr error
		lintSummary, lintErr = runPreBuildLint(ctx, rootDir, ci, color, w)
		output.SectionEnd(w, "sf_lint")
		if lintErr != nil {
			return lintErr
		}
	} else {
		lintSummary = "--skip-lint"
	}

	// --- Detect ---
	output.SectionStartCollapsed(w, "sf_detect", "Detect")
	detectStart := time.Now()

	engine, err := build.Get("image")
	if err != nil {
		output.SectionEnd(w, "sf_detect")
		return err
	}

	det, err := engine.Detect(ctx, rootDir)
	if err != nil {
		output.SectionEnd(w, "sf_detect")
		return fmt.Errorf("detection: %w", err)
	}
	detectElapsed := time.Since(detectStart)

	detectSec := output.NewSection(w, "Detect", detectElapsed, color)
	for _, df := range det.Dockerfiles {
		detectSec.Row("%-16s→ %s", "Dockerfile", df.Path)
	}
	detectSec.Row("%-16s→ %s (auto-detected)", "language", det.Language)
	detectSec.Row("%-16s→ %s", "context", ".")
	if dbTarget != "" {
		detectSec.Row("%-16s→ %s", "target", dbTarget)
	} else {
		detectSec.Row("%-16s→ %s", "target", "(default)")
	}
	detectSec.Close()
	output.SectionEnd(w, "sf_detect")

	detectSummary := fmt.Sprintf("%d Dockerfile(s), %s", len(det.Dockerfiles), det.Language)

	// --- Plan ---
	output.SectionStartCollapsed(w, "sf_plan", "Plan")
	planStart := time.Now()

	// Apply CLI overrides to builds
	planCfg := *cfg
	if dbTarget != "" || len(dbPlatforms) > 0 || dbLocal {
		builds := make([]config.BuildConfig, len(planCfg.Builds))
		copy(builds, planCfg.Builds)
		for i := range builds {
			if builds[i].Kind != "docker" {
				continue
			}
			if dbBuildID != "" && builds[i].ID != dbBuildID {
				continue
			}
			if dbTarget != "" {
				builds[i].Target = dbTarget
			}
			if len(dbPlatforms) > 0 {
				builds[i].Platforms = dbPlatforms
			}
			if dbLocal && len(builds[i].Platforms) == 0 {
				builds[i].Platforms = []string{fmt.Sprintf("linux/%s", runtime.GOARCH)}
			}
		}
		planCfg.Builds = builds
	}

	plan, err := engine.Plan(ctx, &engines.ImagePlanInput{Cfg: &planCfg, BuildID: dbBuildID}, det)
	if err != nil {
		output.SectionEnd(w, "sf_plan")
		return fmt.Errorf("planning: %w", err)
	}

	// Apply CLI tag overrides
	if len(dbTags) > 0 {
		for i := range plan.Steps {
			plan.Steps[i].Tags = append(plan.Steps[i].Tags, dbTags...)
		}
	}

	// Build strategy:
	//   Single-platform: --load into daemon, then docker push each remote tag.
	//     Image exists locally (for retention, scanning, re-tagging) AND remotely.
	//   Multi-platform:  --push directly (can't --load multi-platform in buildx).
	//     No local copy. Remote retention still works.
	//   --local flag:    force --load, no push regardless.
	for i := range plan.Steps {
		step := &plan.Steps[i]
		if dbLocal {
			step.Load = true
			step.Push = false
			if len(step.Tags) == 0 {
				step.Tags = []string{"stagefreight:dev"}
			}
		} else if len(step.Registries) == 0 {
			// No registries configured — load locally
			step.Load = true
			if len(step.Tags) == 0 {
				step.Tags = []string{"stagefreight:dev"}
			}
		} else if build.IsMultiPlatform(*step) {
			// Multi-platform: must --push, can't --load
			step.Push = true
		} else {
			// Single-platform with registries: --load, then push explicitly
			step.Load = true
		}
	}

	planElapsed := time.Since(planStart)

	// Plan summary
	tagCount := 0
	var tagNames []string
	for _, s := range plan.Steps {
		tagCount += len(s.Tags)
		tagNames = append(tagNames, s.Tags...)
	}
	step0 := plan.Steps[0]
	var strategy string
	switch {
	case step0.Push:
		strategy = "multi-platform push"
	case step0.Load && hasRemoteRegistries(step0.Registries):
		strategy = "load + push"
	default:
		strategy = "local"
	}

	planSec := output.NewSection(w, "Plan", planElapsed, color)
	planSec.Row("%-16s%s", "builds", fmt.Sprintf("%d", len(plan.Steps)))
	planSec.Row("%-16s%s", "platforms", formatPlatforms(plan.Steps))
	planSec.Row("%-16s%s", "tags", strings.Join(tagNames, ", "))
	planSec.Row("%-16s%s", "strategy", strategy)
	planSec.Close()
	output.SectionEnd(w, "sf_plan")

	planSummary := fmt.Sprintf("%d build(s), %s, %d tag(s), %s", len(plan.Steps), formatPlatforms(plan.Steps), tagCount, strategy)

	// --- Dry run ---
	if dbDryRun {
		for _, step := range plan.Steps {
			fmt.Printf("step: %s\n", step.Name)
			fmt.Printf("  dockerfile: %s\n", step.Dockerfile)
			fmt.Printf("  context:    %s\n", step.Context)
			fmt.Printf("  target:     %s\n", step.Target)
			fmt.Printf("  platforms:  %v\n", step.Platforms)
			fmt.Printf("  tags:       %v\n", step.Tags)
			fmt.Printf("  load:       %v\n", step.Load)
			fmt.Printf("  push:       %v\n", step.Push)
			if len(step.BuildArgs) > 0 {
				fmt.Printf("  build_args: %v\n", step.BuildArgs)
			}
		}
		return nil
	}

	// --- Execute ---
	output.SectionStart(w, "sf_build", "Build")
	buildStart := time.Now()

	// Always capture output for structured display; verbose forwards stderr in real-time
	bx := build.NewBuildx(verbose)
	var stderrBuf bytes.Buffer
	bx.Stdout = io.Discard
	if verbose {
		bx.Stderr = os.Stderr // BuildWithLayers MultiWriters this + its parse buffer
	} else {
		bx.Stderr = &stderrBuf
	}

	// Login to remote registries (suppress raw output — structured section handles display)
	for _, step := range plan.Steps {
		if hasRemoteRegistries(step.Registries) {
			loginBx := *bx
			loginBx.Stdout = io.Discard
			loginBx.Stderr = io.Discard
			if err := loginBx.Login(ctx, step.Registries); err != nil {
				output.SectionEnd(w, "sf_build")
				return err
			}
			break
		}
	}

	// Build each step — always use BuildWithLayers for structured layer output
	var result build.BuildResult
	for _, step := range plan.Steps {
		stepResult, layers, err := bx.BuildWithLayers(ctx, step)
		if stepResult != nil {
			stepResult.Layers = layers
		}

		result.Steps = append(result.Steps, *stepResult)
		if err != nil {
			// Structured failure: render whatever layers completed
			buildElapsed := time.Since(buildStart)
			failSec := output.NewSection(w, "Build", buildElapsed, color)
			renderBuildLayers(failSec, result.Steps, color)
			output.RowStatus(failSec, "status", "build failed", "failed", color)
			failSec.Close()

			// Raw output: collapsed in CI, shown only if verbose locally
			if ci {
				output.SectionStartCollapsed(w, "sf_build_raw", "Build Output (raw)")
				fmt.Fprint(w, stderrBuf.String())
				output.SectionEnd(w, "sf_build_raw")
			} else if verbose {
				fmt.Fprint(os.Stderr, stderrBuf.String())
			}

			output.SectionEnd(w, "sf_build")
			return err
		}
	}
	buildElapsed := time.Since(buildStart)

	// Build section output
	buildSec := output.NewSection(w, "Build", buildElapsed, color)

	// Render layer events if available
	if renderBuildLayers(buildSec, result.Steps, color) {
		buildSec.Separator()
	}

	var buildImageCount int
	var buildSummaryParts []string
	for _, sr := range result.Steps {
		for _, img := range sr.Images {
			buildSec.Row("result  %-40s", img)
			buildImageCount++
		}
	}
	buildSec.Close()

	buildSummaryParts = append(buildSummaryParts, fmt.Sprintf("%d image(s)", buildImageCount))
	buildSummary := strings.Join(buildSummaryParts, ", ")
	output.SectionEnd(w, "sf_build")

	// --- Push (single-platform load-then-push) ---
	// For single-platform builds that loaded into the daemon, push remote tags now.
	remoteTags := collectRemoteTags(plan)
	var pushSummary string
	var pushElapsed time.Duration
	if len(remoteTags) > 0 {
		output.SectionStart(w, "sf_push", "Push")
		pushStart := time.Now()

		pushBx := *bx
		pushBx.Stdout = io.Discard
		if verbose {
			pushBx.Stderr = os.Stderr
		} else {
			pushBx.Stderr = io.Discard
		}
		if err := pushBx.PushTags(ctx, remoteTags); err != nil {
			pushElapsed = time.Since(pushStart)
			output.SectionEnd(w, "sf_push")
			return err
		}

		pushElapsed = time.Since(pushStart)
		pushSec := output.NewSection(w, "Push", pushElapsed, color)
		for _, tag := range remoteTags {
			pushSec.Row("%-50s %s", tag, output.StatusIcon("success", color))
		}
		pushSec.Close()

		// Count unique registries
		regSet := make(map[string]bool)
		for _, tag := range remoteTags {
			parts := strings.SplitN(tag, "/", 2)
			if len(parts) > 0 {
				regSet[parts[0]] = true
			}
		}
		pushSummary = fmt.Sprintf("%d tag(s) → %d registry", len(remoteTags), len(regSet))
		output.SectionEnd(w, "sf_push")
	}

	// --- Badges ---
	var badgeSummary string
	if hasNarratorBadgeItems() {
		badgeSummary, _ = runBadgeSection(w, color, rootDir)
	}

	// --- README Sync ---
	var readmeSummary string
	readmeTargets := collectTargetsByKind(cfg, "docker-readme")
	if len(readmeTargets) > 0 && !dbLocal {
		readmeSummary, _ = runReadmeSyncSection(ctx, w, ci, color, readmeTargets, rootDir)
	}

	// --- Retention ---
	var retentionSummary string
	if hasRetention(plan) {
		retentionSummary, _ = runRetentionSection(ctx, w, ci, color, plan)
	}

	// --- Summary ---
	totalElapsed := time.Since(pipelineStart)
	overallStatus := "success"

	sumSec := output.NewSection(w, "Summary", 0, color)

	// Lint
	lintStatus := "success"
	if lintSummary == "--skip-lint" {
		lintStatus = "skipped"
	}
	output.SummaryRow(w, "lint", lintStatus, lintSummary, color)

	// Detect
	output.SummaryRow(w, "detect", "success", detectSummary, color)

	// Plan
	output.SummaryRow(w, "plan", "success", planSummary, color)

	// Build
	output.SummaryRow(w, "build", "success", buildSummary, color)

	// Push
	if pushSummary != "" {
		output.SummaryRow(w, "push", "success", pushSummary, color)
	}

	// Badges
	if badgeSummary != "" {
		output.SummaryRow(w, "badges", "success", badgeSummary, color)
	}

	// Readme
	if readmeSummary != "" {
		output.SummaryRow(w, "readme", "success", readmeSummary, color)
	}

	// Retention
	if retentionSummary != "" {
		output.SummaryRow(w, "retention", "success", retentionSummary, color)
	}

	sumSec.Separator()
	output.SummaryTotal(w, totalElapsed, overallStatus, color)
	sumSec.Close()

	// --- Image References ---
	fmt.Fprintf(w, "\n    Image References\n")
	for _, sr := range result.Steps {
		for _, img := range sr.Images {
			fmt.Fprintf(w, "    → %s\n", img)
		}
	}
	fmt.Fprintln(w)

	return nil
}

func runPreBuildLint(ctx context.Context, rootDir string, ci bool, color bool, w io.Writer) (string, error) {
	cacheDir := lint.ResolveCacheDir(rootDir, cfg.Lint.CacheDir)
	cache := &lint.Cache{
		Dir:     cacheDir,
		Enabled: true,
	}

	lintEngine, err := lint.NewEngine(cfg.Lint, rootDir, nil, nil, verbose, cache)
	if err != nil {
		return "", err
	}

	files, err := lintEngine.CollectFiles()
	if err != nil {
		return "", err
	}

	// Delta filtering — skip when config requests full scan.
	if cfg.Lint.Level != config.LevelFull {
		delta := &lint.Delta{RootDir: rootDir, TargetBranch: cfg.Lint.TargetBranch, Verbose: verbose}
		changedSet, _ := delta.ChangedFiles(ctx)
		if changedSet != nil {
			files = lint.FilterByDelta(files, changedSet)
		}
	}

	start := time.Now()
	findings, modStats, runErr := lintEngine.RunWithStats(ctx, files)
	findings = append(findings, modules.CheckFilenameCollisions(files)...)
	elapsed := time.Since(start)

	// Tally
	var critical, warning, info int
	var totalFiles, totalCached int
	for _, f := range findings {
		switch f.Severity {
		case lint.SeverityCritical:
			critical++
		case lint.SeverityWarning:
			warning++
		case lint.SeverityInfo:
			info++
		}
	}
	for _, ms := range modStats {
		totalFiles += ms.Files
		totalCached += ms.Cached
	}

	// Write JUnit XML in CI for GitLab test reporting
	if ci {
		moduleNames := lintEngine.ModuleNames()
		if jErr := output.WriteLintJUnit(".stagefreight/reports", findings, files, moduleNames, elapsed); jErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to write junit report: %v\n", jErr)
		}
	}

	// Section output
	sec := output.NewSection(w, "Lint", elapsed, color)
	output.LintTable(w, modStats, color)
	sec.Separator()
	sec.Row("%-16s%5d   %5d   %d findings (%d critical)",
		"total", totalFiles, totalCached, len(findings), critical)
	sec.Close()

	if len(findings) > 0 {
		fSec := output.NewSection(w, "Findings", 0, color)
		output.SectionFindings(fSec, findings, color)
		fSec.Separator()
		fSec.Row("%s", output.FindingsSummaryLine(len(findings), critical, warning, info, len(files), color))
		fSec.Close()
	}

	if critical > 0 {
		summary := fmt.Sprintf("%d files, %d cached, %d critical", len(files), totalCached, critical)
		return summary, fmt.Errorf("lint failed: %d critical findings", critical)
	}

	summary := fmt.Sprintf("%d files, %d cached, 0 critical", len(files), totalCached)
	if warning > 0 {
		summary = fmt.Sprintf("%d files, %d cached, %d warnings", len(files), totalCached, warning)
	}

	if runErr != nil && verbose {
		fmt.Fprintf(os.Stderr, "lint warning: %v\n", runErr)
	}

	return summary, nil
}

// hasRetention returns true if any step has a registry with retention configured.
func hasRetention(plan *build.BuildPlan) bool {
	for _, step := range plan.Steps {
		if !step.Push {
			continue
		}
		for _, reg := range step.Registries {
			if reg.Retention.Active() {
				return true
			}
		}
	}
	return false
}

// runRetentionSection applies tag retention with section-formatted output.
// Returns a summary string and elapsed time for the summary table.
func runRetentionSection(ctx context.Context, w io.Writer, _ bool, color bool, plan *build.BuildPlan) (string, time.Duration) {
	output.SectionStartCollapsed(w, "sf_retention", "Retention")
	retStart := time.Now()

	var totalDeleted int
	var totalKept int
	var totalErrors int
	var deletedNames []string

	for _, step := range plan.Steps {
		if !step.Push {
			continue
		}
		for _, reg := range step.Registries {
			if !reg.Retention.Active() {
				continue
			}

			client, err := registry.NewRegistry(reg.Provider, reg.URL, reg.Credentials)
			if err != nil {
				totalErrors++
				continue
			}

			result, err := registry.ApplyRetention(ctx, client, reg.Path, reg.TagPatterns, reg.Retention)
			if err != nil {
				totalErrors++
				continue
			}

			totalKept += result.Kept
			totalDeleted += len(result.Deleted)
			totalErrors += len(result.Errors)
			deletedNames = append(deletedNames, result.Deleted...)
		}
	}

	retElapsed := time.Since(retStart)

	sec := output.NewSection(w, "Retention", retElapsed, color)
	for _, step := range plan.Steps {
		for _, reg := range step.Registries {
			if !reg.Retention.Active() {
				continue
			}
			sec.Row("%-40skept %d, pruned %d", reg.URL+"/"+reg.Path, totalKept, totalDeleted)
		}
	}
	for _, d := range deletedNames {
		sec.Row("  - %s", d)
	}
	sec.Close()
	output.SectionEnd(w, "sf_retention")

	summary := fmt.Sprintf("kept %d, pruned %d", totalKept, totalDeleted)
	if totalErrors > 0 {
		summary += fmt.Sprintf(", %d error(s)", totalErrors)
	}

	return summary, retElapsed
}

// hasRemoteRegistries returns true if the registry list has any non-local providers.
func hasRemoteRegistries(registries []build.RegistryTarget) bool {
	for _, r := range registries {
		if r.Provider != "local" {
			return true
		}
	}
	return false
}

// collectRemoteTags returns fully qualified image refs for all remote registry
// tags in load-then-push steps (single-platform, Load=true, has remote registries).
func collectRemoteTags(plan *build.BuildPlan) []string {
	var tags []string
	for _, step := range plan.Steps {
		// Only for load-then-push (single-platform loaded into daemon)
		if !step.Load || step.Push {
			continue
		}
		for _, reg := range step.Registries {
			if reg.Provider == "local" {
				continue
			}
			for _, t := range reg.Tags {
				tags = append(tags, fmt.Sprintf("%s/%s:%s", reg.URL, reg.Path, t))
			}
		}
	}
	return tags
}

func formatPlatforms(steps []build.BuildStep) string {
	if len(steps) == 0 {
		return ""
	}
	// Collect unique platforms across all steps
	seen := make(map[string]bool)
	var platforms []string
	for _, s := range steps {
		for _, p := range s.Platforms {
			if !seen[p] {
				seen[p] = true
				platforms = append(platforms, p)
			}
		}
	}
	if len(platforms) == 0 {
		return runtime.GOOS + "/" + runtime.GOARCH
	}
	return strings.Join(platforms, ",")
}

// buildContextKV returns key-value pairs for the pipeline context block.
func buildContextKV() []output.KV {
	var kv []output.KV

	if pipe := os.Getenv("CI_PIPELINE_ID"); pipe != "" {
		kv = append(kv, output.KV{Key: "Pipeline", Value: pipe})
	}
	if runner := os.Getenv("CI_RUNNER_DESCRIPTION"); runner != "" {
		kv = append(kv, output.KV{Key: "Runner", Value: runner})
	}

	if sha := os.Getenv("CI_COMMIT_SHORT_SHA"); sha != "" {
		kv = append(kv, output.KV{Key: "Commit", Value: sha})
	} else if sha := os.Getenv("CI_COMMIT_SHA"); sha != "" && len(sha) >= 8 {
		kv = append(kv, output.KV{Key: "Commit", Value: sha[:8]})
	}
	if branch := os.Getenv("CI_COMMIT_BRANCH"); branch != "" {
		kv = append(kv, output.KV{Key: "Branch", Value: branch})
	} else if tag := os.Getenv("CI_COMMIT_TAG"); tag != "" {
		kv = append(kv, output.KV{Key: "Tag", Value: tag})
	}

	platforms := formatPlatforms(nil) // filled after plan, but context block is pre-plan
	if p := os.Getenv("STAGEFREIGHT_PLATFORMS"); p != "" {
		platforms = p
	}
	if platforms != "" {
		kv = append(kv, output.KV{Key: "Platforms", Value: platforms})
	}

	// Count configured registry targets
	regTargets := collectTargetsByKind(cfg, "registry")
	if len(regTargets) > 0 {
		var regNames []string
		seen := make(map[string]bool)
		for _, t := range regTargets {
			if !seen[t.URL] {
				regNames = append(regNames, t.URL)
				seen[t.URL] = true
			}
		}
		kv = append(kv, output.KV{Key: "Registries", Value: fmt.Sprintf("%d (%s)", len(regTargets), strings.Join(regNames, ", "))})
	}

	return kv
}

// hasNarratorBadgeItems returns true if any narrator item has badge generation configured.
func hasNarratorBadgeItems() bool {
	for _, f := range cfg.Narrator {
		for _, item := range f.Items {
			if item.HasGeneration() {
				return true
			}
		}
	}
	return false
}

// collectNarratorBadgeItems returns all narrator items with badge generation.
func collectNarratorBadgeItems() []config.NarratorItem {
	var items []config.NarratorItem
	for _, f := range cfg.Narrator {
		for _, item := range f.Items {
			if item.HasGeneration() {
				items = append(items, item)
			}
		}
	}
	return items
}

// runBadgeSection generates configured badges with section-formatted output.
func runBadgeSection(w io.Writer, color bool, rootDir string) (string, time.Duration) {
	output.SectionStartCollapsed(w, "sf_badges", "Badges")
	start := time.Now()

	eng, err := buildDefaultBadgeEngine()
	if err != nil {
		elapsed := time.Since(start)
		sec := output.NewSection(w, "Badges", elapsed, color)
		sec.Row("error: %v", err)
		sec.Close()
		output.SectionEnd(w, "sf_badges")
		return fmt.Sprintf("error: %v", err), elapsed
	}

	items := collectNarratorBadgeItems()

	// Detect version for template resolution
	vi, _ := build.DetectVersion(rootDir)

	// Lazy Docker Hub info — only fetch if any badge value uses {docker.*}
	var dockerInfo *gitver.DockerHubInfo
	for _, item := range items {
		if strings.Contains(item.Value, "{docker.") {
			ns, repo := dockerHubFromConfig()
			if ns != "" && repo != "" {
				dockerInfo, _ = gitver.FetchDockerHubInfo(ns, repo)
			}
			break
		}
	}

	var generated int
	for _, item := range items {
		spec := item.ToBadgeSpec()

		// Per-item engine if font is overridden
		itemEng := eng
		if spec.Font != "" || spec.FontFile != "" || spec.FontSize != 0 {
			override, oErr := buildItemEngine(spec)
			if oErr != nil {
				continue
			}
			itemEng = override
		}

		// Resolve value templates
		value := spec.Value
		if vi != nil && value != "" {
			value = gitver.ResolveTemplateWithDirAndVars(value, vi, rootDir, cfg.Vars)
		}
		value = gitver.ResolveDockerTemplates(value, dockerInfo)

		// Resolve color
		badgeColor := spec.Color
		if badgeColor == "" || badgeColor == "auto" {
			badgeColor = badge.StatusColor("passed")
		}

		svg := itemEng.Generate(badge.Badge{
			Label: spec.Label,
			Value: value,
			Color: badgeColor,
		})

		if mkErr := os.MkdirAll(filepath.Dir(spec.Output), 0o755); mkErr != nil {
			continue
		}
		if wErr := os.WriteFile(spec.Output, []byte(svg), 0o644); wErr != nil {
			continue
		}
		generated++
	}

	elapsed := time.Since(start)
	sec := output.NewSection(w, "Badges", elapsed, color)
	for _, item := range items {
		spec := item.ToBadgeSpec()
		fontName := spec.Font
		if fontName == "" {
			fontName = "dejavu-sans"
		}
		size := spec.FontSize
		if size == 0 {
			size = 11
		}
		badgeColor := spec.Color
		if badgeColor == "" {
			badgeColor = "auto"
		}
		sec.Row("%-16s%-24s %-8s %.0fpt  %s", item.Text, spec.Output, fontName, size, badgeColor)
	}
	sec.Close()
	output.SectionEnd(w, "sf_badges")

	summary := fmt.Sprintf("%d generated", generated)
	return summary, elapsed
}

// runReadmeSyncSection syncs README to docker-readme targets with section-formatted output.
func runReadmeSyncSection(ctx context.Context, w io.Writer, _ bool, color bool, targets []config.TargetConfig, rootDir string) (string, time.Duration) {
	output.SectionStartCollapsed(w, "sf_readme", "README Sync")
	start := time.Now()

	var synced, errors int

	for _, t := range targets {
		file := t.File
		if file == "" {
			file = "README.md"
		}

		content, err := registry.PrepareReadmeFromFile(file, t.Description, t.LinkBase, rootDir)
		if err != nil {
			errors++
			continue
		}

		provider := t.Provider
		if provider == "" {
			provider = build.DetectProvider(t.URL)
		}

		client, err := registry.NewRegistry(provider, t.URL, t.Credentials)
		if err != nil {
			errors++
			continue
		}

		short := content.Short
		if t.Description != "" {
			short = t.Description
		}

		if err := client.UpdateDescription(ctx, t.Path, short, content.Full); err != nil {
			errors++
			continue
		}
		synced++
	}

	elapsed := time.Since(start)
	sec := output.NewSection(w, "Readme", elapsed, color)
	for _, t := range targets {
		sec.Row("%-40ssynced", t.URL+"/"+t.Path)
	}
	sec.Close()
	output.SectionEnd(w, "sf_readme")

	summary := fmt.Sprintf("%d synced", synced)
	if errors > 0 {
		summary += fmt.Sprintf(", %d error(s)", errors)
	}
	return summary, elapsed
}

// renderBuildLayers renders parsed layer events into a Section.
// Returns true if any layers were rendered.
func renderBuildLayers(sec *output.Section, steps []build.StepResult, color bool) bool {
	hasLayers := false
	for _, sr := range steps {
		for _, layer := range sr.Layers {
			instr := build.FormatLayerInstruction(layer)
			timing := build.FormatLayerTiming(layer)

			var label string
			if layer.Instruction == "FROM" {
				label = "base"
			} else {
				label = layer.Instruction
			}

			timingStr := timing
			if layer.Cached {
				timingStr = output.Dimmed("cached", color)
			}
			sec.Row("%-8s%-42s %s", label, instr, timingStr)
			hasLayers = true
		}
	}
	return hasLayers
}

// dockerHubFromConfig returns the namespace and repo for the first docker.io registry target.
func dockerHubFromConfig() (string, string) {
	for _, t := range cfg.Targets {
		if t.Kind == "registry" && t.URL == "docker.io" && t.Path != "" {
			resolved := gitver.ResolveVars(t.Path, cfg.Vars)
			parts := strings.SplitN(resolved, "/", 2)
			if len(parts) == 2 {
				return parts[0], parts[1]
			}
		}
	}
	return "", ""
}

// collectTargetsByKind returns all targets matching the given kind.
func collectTargetsByKind(cfg *config.Config, kind string) []config.TargetConfig {
	var targets []config.TargetConfig
	for _, t := range cfg.Targets {
		if t.Kind == kind {
			targets = append(targets, t)
		}
	}
	return targets
}

// firstDockerReadmeDescription returns the description from the first docker-readme target.
func firstDockerReadmeDescription(cfg *config.Config) string {
	for _, t := range cfg.Targets {
		if t.Kind == "docker-readme" && t.Description != "" {
			return t.Description
		}
	}
	return ""
}
