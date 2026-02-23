package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/sofmeright/stagefreight/src/build"
	_ "github.com/sofmeright/stagefreight/src/build/engines"
	"github.com/sofmeright/stagefreight/src/lint"
	"github.com/sofmeright/stagefreight/src/lint/modules"
	"github.com/sofmeright/stagefreight/src/output"
	"github.com/sofmeright/stagefreight/src/registry"
)

var (
	dbLocal     bool
	dbPlatforms []string
	dbTags      []string
	dbTarget    string
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
	w := os.Stdout

	// CI header with pipeline context
	output.CIHeader(w)

	// --- Pre-build lint gate ---
	if !dbSkipLint {
		output.SectionStart(w, "sf_lint", "Lint")
		lintErr := runPreBuildLint(ctx, rootDir, ci)
		output.SectionEnd(w, "sf_lint")
		if lintErr != nil {
			return lintErr
		}
	} else if ci {
		output.PhaseResult(w, "lint", "skipped", "--skip-lint", 0)
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

	if verbose {
		fmt.Fprintf(os.Stderr, "detected: %d Dockerfiles, language=%s\n",
			len(det.Dockerfiles), det.Language)
	}

	output.PhaseResult(w, "detect", "success",
		fmt.Sprintf("%d Dockerfile(s), %s", len(det.Dockerfiles), det.Language),
		detectElapsed)
	output.SectionEnd(w, "sf_detect")

	// --- Plan ---
	output.SectionStartCollapsed(w, "sf_plan", "Plan")
	planStart := time.Now()

	dockerCfg := cfg.Docker

	// Apply CLI overrides
	if dbTarget != "" {
		dockerCfg.Target = dbTarget
	}
	if len(dbPlatforms) > 0 {
		dockerCfg.Platforms = dbPlatforms
	}
	if dbLocal {
		if len(dockerCfg.Platforms) == 0 {
			dockerCfg.Platforms = []string{fmt.Sprintf("linux/%s", runtime.GOARCH)}
		}
	}

	plan, err := engine.Plan(ctx, &dockerCfg, det)
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
	var planParts []string
	planParts = append(planParts, formatPlatforms(plan.Steps))
	tagCount := 0
	for _, s := range plan.Steps {
		tagCount += len(s.Tags)
	}
	step0 := plan.Steps[0]
	switch {
	case step0.Push:
		planParts = append(planParts, fmt.Sprintf("%d tag(s), multi-platform push", tagCount))
	case step0.Load && hasRemoteRegistries(step0.Registries):
		planParts = append(planParts, fmt.Sprintf("%d tag(s), load+push", tagCount))
	default:
		planParts = append(planParts, fmt.Sprintf("%d tag(s), local", tagCount))
	}
	output.PhaseResult(w, "plan", "success", strings.Join(planParts, ", "), planElapsed)
	output.SectionEnd(w, "sf_plan")

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

	// In CI, capture buildx output to suppress noise
	bx := build.NewBuildx(verbose)
	if ci && !verbose {
		bx.Stdout = &bytes.Buffer{}
		bx.Stderr = &bytes.Buffer{}
	}

	// Login to remote registries (skips local providers)
	for _, step := range plan.Steps {
		if hasRemoteRegistries(step.Registries) {
			if err := bx.Login(ctx, step.Registries); err != nil {
				output.SectionEnd(w, "sf_build")
				return err
			}
			break
		}
	}

	// Build each step
	var result build.BuildResult
	for _, step := range plan.Steps {
		stepResult, err := bx.Build(ctx, step)
		result.Steps = append(result.Steps, *stepResult)
		if err != nil {
			buildElapsed := time.Since(buildStart)
			if ci && !verbose {
				if buf, ok := bx.Stderr.(*bytes.Buffer); ok && buf.Len() > 0 {
					fmt.Fprint(w, buf.String())
				}
			}
			output.PhaseResult(w, "build", "failed", err.Error(), buildElapsed)
			output.SectionEnd(w, "sf_build")
			return err
		}
	}
	buildElapsed := time.Since(buildStart)

	// Build summary
	var buildDetail []string
	buildDetail = append(buildDetail, formatPlatforms(plan.Steps))
	for _, sr := range result.Steps {
		for _, img := range sr.Images {
			parts := strings.Split(img, "/")
			buildDetail = append(buildDetail, parts[len(parts)-1])
			break
		}
	}
	output.PhaseResult(w, "build", "success", strings.Join(buildDetail, ", "), buildElapsed)
	output.SectionEnd(w, "sf_build")

	// --- Push (single-platform load-then-push) ---
	// For single-platform builds that loaded into the daemon, push remote tags now.
	remoteTags := collectRemoteTags(plan)
	if len(remoteTags) > 0 {
		output.SectionStart(w, "sf_push", "Push")
		pushStart := time.Now()

		if err := bx.PushTags(ctx, remoteTags); err != nil {
			pushElapsed := time.Since(pushStart)
			output.PhaseResult(w, "push", "failed", err.Error(), pushElapsed)
			output.SectionEnd(w, "sf_push")
			return err
		}

		pushElapsed := time.Since(pushStart)
		output.PhaseResult(w, "push", "success",
			fmt.Sprintf("%d tag(s) pushed", len(remoteTags)), pushElapsed)
		output.SectionEnd(w, "sf_push")
	}

	// --- Retention ---
	if hasRetention(plan) {
		retentionErr := runRetention(ctx, w, ci, plan)
		if retentionErr != nil && verbose {
			fmt.Fprintf(os.Stderr, "retention warning: %v\n", retentionErr)
		}
	}

	// --- Report (non-CI gets full image list) ---
	if !ci {
		for _, sr := range result.Steps {
			for _, img := range sr.Images {
				fmt.Printf("  → %s\n", img)
			}
		}
	} else {
		// CI: print all pushed image refs for easy copy
		output.SectionStartCollapsed(w, "sf_images", "Image References")
		for _, sr := range result.Steps {
			for _, img := range sr.Images {
				fmt.Fprintf(w, "  → %s\n", img)
			}
		}
		output.SectionEnd(w, "sf_images")
	}

	return nil
}

func runPreBuildLint(ctx context.Context, rootDir string, ci bool) error {
	cache := &lint.Cache{
		RootDir: rootDir,
		Enabled: true,
	}

	engine, err := lint.NewEngine(cfg.Lint, rootDir, nil, nil, verbose, cache)
	if err != nil {
		return err
	}

	files, err := engine.CollectFiles()
	if err != nil {
		return err
	}

	// Delta filtering
	delta := &lint.Delta{RootDir: rootDir, Verbose: verbose}
	changedSet, _ := delta.ChangedFiles(ctx)
	if changedSet != nil {
		files = lint.FilterByDelta(files, changedSet)
	}

	start := time.Now()
	findings, runErr := engine.Run(ctx, files)
	findings = append(findings, modules.CheckFilenameCollisions(files)...)
	elapsed := time.Since(start)

	// Tally
	var critical, warning int
	for _, f := range findings {
		switch f.Severity {
		case lint.SeverityCritical:
			critical++
		case lint.SeverityWarning:
			warning++
		}
	}

	cached := engine.CacheHits.Load()

	// Write JUnit XML in CI for GitLab test reporting
	if ci {
		moduleNames := engine.ModuleNames()
		if jErr := output.WriteLintJUnit(".stagefreight/reports", findings, files, moduleNames, elapsed); jErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to write junit report: %v\n", jErr)
		}
	}

	if critical > 0 {
		printer := output.NewPrinter()
		printer.Print(findings)
		detail := fmt.Sprintf("%d critical, %d warning in %d files", critical, warning, len(files))
		output.PhaseResult(os.Stdout, "lint", "failed", detail, elapsed)
		return fmt.Errorf("lint failed: %d critical findings", critical)
	}

	detail := fmt.Sprintf("%d files, %d cached, 0 critical", len(files), cached)
	if warning > 0 {
		detail = fmt.Sprintf("%d files, %d cached, %d warnings", len(files), cached, warning)
	}
	output.PhaseResult(os.Stdout, "lint", "success", detail, elapsed)

	if runErr != nil && verbose {
		fmt.Fprintf(os.Stderr, "lint warning: %v\n", runErr)
	}

	return nil
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

// runRetention applies tag retention for all registries that have it configured.
// Runs after a successful push. Errors are logged but do not fail the build.
func runRetention(ctx context.Context, w io.Writer, ci bool, plan *build.BuildPlan) error {
	output.SectionStartCollapsed(w, "sf_retention", "Retention")
	retStart := time.Now()

	var totalDeleted int
	var totalErrors int

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
				fmt.Fprintf(w, "  retention: skip %s/%s: %v\n", reg.URL, reg.Path, err)
				totalErrors++
				continue
			}

			result, err := registry.ApplyRetention(ctx, client, reg.Path, reg.TagPatterns, reg.Retention)
			if err != nil {
				fmt.Fprintf(w, "  retention: %s/%s: %v\n", reg.URL, reg.Path, err)
				totalErrors++
				continue
			}

			if len(result.Deleted) > 0 {
				fmt.Fprintf(w, "  retention: %s/%s: matched=%d kept=%d deleted=%d\n",
					reg.URL, reg.Path, result.Matched, result.Kept, len(result.Deleted))
				for _, d := range result.Deleted {
					fmt.Fprintf(w, "    - %s\n", d)
				}
			}
			totalDeleted += len(result.Deleted)
			totalErrors += len(result.Errors)
		}
	}

	retElapsed := time.Since(retStart)

	detail := fmt.Sprintf("deleted %d tag(s)", totalDeleted)
	if totalErrors > 0 {
		detail += fmt.Sprintf(", %d error(s)", totalErrors)
	}
	if totalDeleted == 0 && totalErrors == 0 {
		detail = "nothing to prune"
	}

	status := "success"
	if totalErrors > 0 && totalDeleted == 0 {
		status = "failed"
	}

	output.PhaseResult(w, "retention", status, detail, retElapsed)
	output.SectionEnd(w, "sf_retention")

	return nil
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
	platforms := steps[0].Platforms
	if len(platforms) == 0 {
		return runtime.GOOS + "/" + runtime.GOARCH
	}
	result := ""
	for i, p := range platforms {
		if i > 0 {
			result += ","
		}
		result += p
	}
	return result
}
