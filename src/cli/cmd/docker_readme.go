package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/sofmeright/stagefreight/src/config"
	"github.com/sofmeright/stagefreight/src/output"
	"github.com/sofmeright/stagefreight/src/registry"
)

var drDryRun bool

var dockerReadmeCmd = &cobra.Command{
	Use:   "readme",
	Short: "Sync README to container registries",
	Long: `Push README content to container registries that support description APIs.

Docker Hub receives both short (100-char) and full markdown descriptions.
Quay and Harbor receive short descriptions only.
Other registries are silently skipped.`,
	RunE: runDockerReadme,
}

func init() {
	dockerReadmeCmd.Flags().BoolVar(&drDryRun, "dry-run", false, "show prepared content without pushing")
	dockerCmd.AddCommand(dockerReadmeCmd)
}

// readmeSyncResult tracks the outcome of syncing to a single registry.
type readmeSyncResult struct {
	Registry string
	Status   string // "success" | "skipped" | "failed"
	Detail   string
	Err      error
}

func runDockerReadme(cmd *cobra.Command, args []string) error {
	rootDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	if len(args) > 0 {
		rootDir = args[0]
	}

	if !cfg.Docker.Readme.IsActive() {
		return fmt.Errorf("no readme configuration found in docker.readme")
	}

	ctx := context.Background()
	color := output.UseColor()
	w := os.Stdout

	content, err := registry.PrepareReadme(cfg.Docker.Readme, rootDir)
	if err != nil {
		return err
	}

	if drDryRun {
		fmt.Fprintf(w, "Short description (%d chars):\n  %s\n\n", len(content.Short), content.Short)
		fmt.Fprintf(w, "Full description (%d bytes):\n%s\n", len(content.Full), content.Full)
		return nil
	}

	start := time.Now()
	results := syncReadmeCollect(ctx, cfg.Docker.Readme, cfg.Docker.Registries, content)
	elapsed := time.Since(start)

	// Tally
	var synced, skipped, errors int
	for _, r := range results {
		switch r.Status {
		case "success":
			synced++
		case "skipped":
			skipped++
		case "failed":
			errors++
		}
	}

	// ── README Sync section ──
	output.SectionStart(w, "sf_readme", "README Sync")
	sec := output.NewSection(w, "README Sync", elapsed, color)

	for _, r := range results {
		output.RowStatus(sec, r.Registry, r.Detail, r.Status, color)
	}

	sec.Separator()
	sec.Row("%d synced, %d skipped, %d errors", synced, skipped, errors)

	sec.Close()
	output.SectionEnd(w, "sf_readme")

	if errors > 0 {
		return fmt.Errorf("readme sync had %d error(s)", errors)
	}
	return nil
}

// syncReadmeCollect pushes README content to all supported registries,
// returning results for each. Used by standalone `docker readme` command.
func syncReadmeCollect(ctx context.Context, readmeCfg config.DockerReadmeConfig, registries []config.RegistryConfig, content *registry.ReadmeContent) []readmeSyncResult {
	var results []readmeSyncResult
	seen := make(map[string]bool)

	for _, reg := range registries {
		name := reg.URL + "/" + reg.Path

		if seen[name] {
			continue
		}
		seen[name] = true

		provider := reg.Provider
		if provider == "" {
			provider = "generic"
		} else {
			var err error
			provider, err = registry.CanonicalProvider(provider)
			if err != nil {
				results = append(results, readmeSyncResult{Registry: name, Status: "failed", Detail: err.Error()})
				continue
			}
		}

		// Only docker, github, quay, harbor support description APIs
		switch provider {
		case "docker", "github", "quay", "harbor":
			// supported
		default:
			results = append(results, readmeSyncResult{Registry: name, Status: "skipped", Detail: "no description API"})
			continue
		}

		client, err := registry.NewRegistry(provider, reg.URL, reg.Credentials)
		if err != nil {
			results = append(results, readmeSyncResult{Registry: name, Status: "failed", Detail: err.Error(), Err: err})
			fmt.Fprintf(os.Stderr, "readme: error %s: %v\n", name, err)
			continue
		}

		// Per-registry description override
		short := content.Short
		if reg.Description != "" {
			short = reg.Description
		}

		if err := client.UpdateDescription(ctx, reg.Path, short, content.Full); err != nil {
			results = append(results, readmeSyncResult{Registry: name, Status: "failed", Detail: err.Error(), Err: err})
			fmt.Fprintf(os.Stderr, "readme: error %s: %v\n", name, err)
			continue
		}

		results = append(results, readmeSyncResult{Registry: name, Status: "success"})
	}

	return results
}

// syncReadmeToRegistries pushes README content to all supported registries.
// Returns counts of synced, skipped, and errored registries.
// Used by the docker build pipeline — left untouched for pipeline compatibility.
func syncReadmeToRegistries(ctx context.Context, w io.Writer, readmeCfg config.DockerReadmeConfig, registries []config.RegistryConfig, content *registry.ReadmeContent) (synced, skipped, errors int) {
	seen := make(map[string]bool)
	for _, reg := range registries {
		name := reg.URL + "/" + reg.Path
		if seen[name] {
			continue
		}
		seen[name] = true

		provider := reg.Provider
		if provider == "" {
			provider = "generic"
		} else {
			var err error
			provider, err = registry.CanonicalProvider(provider)
			if err != nil {
				fmt.Fprintf(w, "  readme: invalid provider %s/%s: %v\n", reg.URL, reg.Path, err)
				errors++
				continue
			}
		}

		// Only docker, github, quay, harbor support description APIs
		switch provider {
		case "docker", "github", "quay", "harbor":
			// supported
		default:
			skipped++
			if verbose {
				fmt.Fprintf(w, "  readme: skip %s/%s (%s: no description API)\n", reg.URL, reg.Path, provider)
			}
			continue
		}

		client, err := registry.NewRegistry(provider, reg.URL, reg.Credentials)
		if err != nil {
			fmt.Fprintf(w, "  readme: error %s/%s: %v\n", reg.URL, reg.Path, err)
			errors++
			continue
		}

		// Per-registry description override
		short := content.Short
		if reg.Description != "" {
			short = reg.Description
		}

		if err := client.UpdateDescription(ctx, reg.Path, short, content.Full); err != nil {
			fmt.Fprintf(w, "  readme: error %s/%s: %v\n", reg.URL, reg.Path, err)
			errors++
			continue
		}

		fmt.Fprintf(w, "  readme: synced %s/%s\n", reg.URL, reg.Path)
		synced++
	}
	return
}

// runReadmeSync is the auto-sync entry point called from docker build.
// Non-fatal: errors are warnings that never fail the build.
func runReadmeSync(ctx context.Context, w io.Writer, ci bool, readmeCfg config.DockerReadmeConfig, registries []config.RegistryConfig, rootDir string) {
	output.SectionStartCollapsed(w, "sf_readme", "README Sync")
	start := time.Now()

	content, err := registry.PrepareReadme(readmeCfg, rootDir)
	if err != nil {
		elapsed := time.Since(start)
		output.PhaseResult(w, "readme", "failed", err.Error(), elapsed)
		output.SectionEnd(w, "sf_readme")
		return
	}

	synced, skipped, errors := syncReadmeToRegistries(ctx, w, readmeCfg, registries, content)
	elapsed := time.Since(start)

	detail := fmt.Sprintf("synced %d, skipped %d", synced, skipped)
	status := "success"
	if errors > 0 {
		detail += fmt.Sprintf(", %d error(s)", errors)
		status = "failed"
	}

	output.PhaseResult(w, "readme", status, detail, elapsed)
	output.SectionEnd(w, "sf_readme")
}
