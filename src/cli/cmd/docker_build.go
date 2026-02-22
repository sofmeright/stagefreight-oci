package cmd

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/spf13/cobra"
	"gitlab.prplanit.com/precisionplanit/stagefreight-oci/src/build"
	_ "gitlab.prplanit.com/precisionplanit/stagefreight-oci/src/build/engines"
	"gitlab.prplanit.com/precisionplanit/stagefreight-oci/src/lint"
	"gitlab.prplanit.com/precisionplanit/stagefreight-oci/src/lint/modules"
	"gitlab.prplanit.com/precisionplanit/stagefreight-oci/src/output"
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

	// --- Pre-build lint gate ---
	if !dbSkipLint {
		if err := runPreBuildLint(ctx, rootDir); err != nil {
			fmt.Fprintf(os.Stderr, "  build skipped — lint failed\n")
			return err
		}
	}

	// --- Detect ---
	engine, err := build.Get("image")
	if err != nil {
		return err
	}

	det, err := engine.Detect(ctx, rootDir)
	if err != nil {
		return fmt.Errorf("detection: %w", err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "detected: %d Dockerfiles, language=%s\n",
			len(det.Dockerfiles), det.Language)
	}

	// --- Plan ---
	dockerCfg := cfg.Docker

	// Apply CLI overrides
	if dbTarget != "" {
		dockerCfg.Target = dbTarget
	}
	if len(dbPlatforms) > 0 {
		dockerCfg.Platforms = dbPlatforms
	}
	if dbLocal {
		// Local mode: single platform, load into daemon, no push
		if len(dockerCfg.Platforms) == 0 {
			dockerCfg.Platforms = []string{fmt.Sprintf("linux/%s", runtime.GOARCH)}
		}
	}

	plan, err := engine.Plan(ctx, &dockerCfg, det)
	if err != nil {
		return fmt.Errorf("planning: %w", err)
	}

	// Apply CLI tag overrides
	if len(dbTags) > 0 {
		for i := range plan.Steps {
			plan.Steps[i].Tags = append(plan.Steps[i].Tags, dbTags...)
		}
	}

	// Set load/push flags
	for i := range plan.Steps {
		if dbLocal {
			plan.Steps[i].Load = true
			plan.Steps[i].Push = false
			// Ensure at least a dev tag for local builds
			if len(plan.Steps[i].Tags) == 0 {
				plan.Steps[i].Tags = []string{"stagefreight:dev"}
			}
		} else if len(plan.Steps[i].Registries) > 0 {
			plan.Steps[i].Push = true
		} else {
			// No registries configured and not --local — default to local
			plan.Steps[i].Load = true
			if len(plan.Steps[i].Tags) == 0 {
				plan.Steps[i].Tags = []string{"stagefreight:dev"}
			}
		}
	}

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
	result, err := engine.Execute(ctx, plan)
	if err != nil {
		return err
	}

	// --- Report ---
	for _, sr := range result.Steps {
		switch sr.Status {
		case "success":
			fmt.Printf("  build %s (%s, %s)\n", colorGreen("✓"), formatPlatforms(plan.Steps), sr.Duration.Round(100*time.Millisecond))
		case "failed":
			fmt.Printf("  build %s (%s)\n", colorRed("✗"), sr.Duration.Round(100*time.Millisecond))
		}
		for _, img := range sr.Images {
			fmt.Printf("  → %s\n", img)
		}
	}

	return nil
}

func runPreBuildLint(ctx context.Context, rootDir string) error {
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
	allCount := len(files)
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

	if critical > 0 {
		printer := output.NewPrinter()
		printer.Print(findings)
		fmt.Fprintf(os.Stdout, "  lint %s %d issues in %d files\n", colorRed("✗"), critical+warning, len(files))
		return fmt.Errorf("lint failed: %d critical findings", critical)
	}

	fmt.Printf("  lint %s (%d files changed, %d cached, %s)\n",
		colorGreen("✓"),
		len(files),
		cached,
		elapsed.Round(100*time.Millisecond),
	)

	if runErr != nil && verbose {
		fmt.Fprintf(os.Stderr, "lint warning: %v\n", runErr)
	}

	_ = allCount // used for verbose reporting
	return nil
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

func colorGreen(s string) string {
	return "\033[32m" + s + "\033[0m"
}

func colorRed(s string) string {
	return "\033[31m" + s + "\033[0m"
}
