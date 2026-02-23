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

	synced, skipped, errs := syncReadmeToRegistries(ctx, w, cfg.Docker.Readme, cfg.Docker.Registries, content)

	fmt.Fprintf(w, "\nreadme: synced=%d skipped=%d errors=%d\n", synced, skipped, errs)
	if errs > 0 {
		return fmt.Errorf("readme sync had %d error(s)", errs)
	}
	return nil
}

// syncReadmeToRegistries pushes README content to all supported registries.
// Returns counts of synced, skipped, and errored registries.
func syncReadmeToRegistries(ctx context.Context, w io.Writer, readmeCfg config.DockerReadmeConfig, registries []config.RegistryConfig, content *registry.ReadmeContent) (synced, skipped, errors int) {
	for _, reg := range registries {
		provider := reg.Provider
		if provider == "" {
			provider = "generic"
		}

		// Only dockerhub, quay, harbor support description APIs
		switch provider {
		case "dockerhub", "quay", "harbor":
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
