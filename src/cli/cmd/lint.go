package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gitlab.prplanit.com/precisionplanit/stagefreight-oci/src/lint"
	"gitlab.prplanit.com/precisionplanit/stagefreight-oci/src/lint/modules"
	"gitlab.prplanit.com/precisionplanit/stagefreight-oci/src/output"
)

var (
	lintLevel    string
	lintModules  []string
	lintNoModule []string
	lintNoCache  bool
	lintAll      bool
)

var lintCmd = &cobra.Command{
	Use:   "lint [paths...]",
	Short: "Run code quality checks",
	Long: `Run cache-aware, delta-only code quality checks.

By default, only changed files are scanned (--level changed).
Use --level full or --all to scan everything.

Modules run in parallel and results are cached by content hash.`,
	RunE: runLint,
}

func init() {
	lintCmd.Flags().StringVar(&lintLevel, "level", "changed", "scan level: changed (default) or full")
	lintCmd.Flags().StringSliceVar(&lintModules, "module", nil, "run only these modules (comma-separated)")
	lintCmd.Flags().StringSliceVar(&lintNoModule, "no-module", nil, "skip these modules (comma-separated)")
	lintCmd.Flags().BoolVar(&lintNoCache, "no-cache", false, "disable cache (clear and rescan)")
	lintCmd.Flags().BoolVar(&lintAll, "all", false, "scan all files (shorthand for --level full)")

	rootCmd.AddCommand(lintCmd)
}

func runLint(cmd *cobra.Command, args []string) error {
	if lintAll {
		lintLevel = "full"
	}

	rootDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	if len(args) > 0 {
		rootDir = args[0]
	}

	// Set up cache
	cache := &lint.Cache{
		RootDir: rootDir,
		Enabled: !lintNoCache,
	}
	if lintNoCache {
		if err := cache.Clear(); err != nil && verbose {
			fmt.Fprintf(os.Stderr, "cache: clear failed: %v\n", err)
		}
	}

	// Ensure .stagefreight/ is in .gitignore
	lint.EnsureGitignore(rootDir)

	engine, err := lint.NewEngine(cfg.Lint, rootDir, lintModules, lintNoModule, verbose, cache)
	if err != nil {
		return err
	}

	if verbose {
		names := make([]string, len(engine.Modules))
		for i, m := range engine.Modules {
			names[i] = m.Name()
		}
		fmt.Fprintf(os.Stderr, "modules: %v\n", names)
	}

	// Collect all files
	files, err := engine.CollectFiles()
	if err != nil {
		return fmt.Errorf("collecting files: %w", err)
	}

	// Delta filtering — only scan changed files unless --level full
	if lintLevel != "full" {
		delta := &lint.Delta{RootDir: rootDir, Verbose: verbose}
		deltaCtx := context.Background()
		changedSet, deltaErr := delta.ChangedFiles(deltaCtx)
		if deltaErr != nil && verbose {
			fmt.Fprintf(os.Stderr, "delta: %v, falling back to full scan\n", deltaErr)
		}
		if changedSet != nil {
			allFiles := files
			files = lint.FilterByDelta(files, changedSet)
			if verbose {
				fmt.Fprintf(os.Stderr, "delta: %d/%d files changed\n", len(files), len(allFiles))
			}
		}
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "scanning %d files\n", len(files))
	}

	ctx := context.Background()
	findings, runErr := engine.Run(ctx, files)

	// Cross-file checks (filename collisions)
	findings = append(findings, modules.CheckFilenameCollisions(files)...)

	printer := output.NewPrinter()
	hasCritical := printer.Print(findings)

	// Tally
	var critical, warning, info int
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
	printer.Summary(len(findings), critical, warning, info, len(files))

	// Cache stats
	if verbose && cache.Enabled {
		fmt.Fprintf(os.Stderr, "cache: %d hits, %d misses\n",
			engine.CacheHits.Load(), engine.CacheMisses.Load())
	}

	if runErr != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", runErr)
	}

	if hasCritical {
		return fmt.Errorf("lint failed: %d critical findings", critical)
	}

	return nil
}
