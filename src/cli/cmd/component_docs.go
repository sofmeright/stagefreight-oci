package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/sofmeright/stagefreight/src/component"
	"github.com/sofmeright/stagefreight/src/forge"
)

var (
	cdSpecs      []string
	cdOutputFile string
	cdCommit     bool
	cdBranch     string
)

var componentDocsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Generate input documentation from component spec files",
	Long: `Parse GitLab CI component spec files and generate markdown
documentation tables for their inputs.

Supports custom group metadata via comments:
  # input_section_name- Group Title
  # input_section_desc- Group description text

Output modes:
  - Default: print markdown to stdout
  - --output: write markdown to a file
  - --commit: commit updated README via forge API (no local clone needed)`,
	RunE: runComponentDocs,
}

func init() {
	componentDocsCmd.Flags().StringSliceVar(&cdSpecs, "spec", nil, "component spec file(s) to parse (repeatable)")
	componentDocsCmd.Flags().StringVarP(&cdOutputFile, "output", "o", "", "write docs to file")
	componentDocsCmd.Flags().BoolVar(&cdCommit, "commit", false, "commit updated README via forge API")
	componentDocsCmd.Flags().StringVar(&cdBranch, "branch", "", "branch to commit to")

	componentCmd.AddCommand(componentDocsCmd)
}

func runComponentDocs(cmd *cobra.Command, args []string) error {
	// Resolve spec files: CLI flags → config → error.
	specFiles := cdSpecs
	if len(specFiles) == 0 {
		specFiles = cfg.GitlabComponent.SpecFiles
	}
	if len(specFiles) == 0 {
		return fmt.Errorf("no spec files specified; use --spec or configure gitlab_component.spec_files in .stagefreight.yml")
	}

	// Parse all spec files.
	var specs []*component.SpecFile
	for _, path := range specFiles {
		spec, err := component.ParseSpec(path)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		specs = append(specs, spec)
		if verbose {
			fmt.Fprintf(os.Stderr, "  parsed %s: %d inputs\n", path, len(spec.Inputs))
		}
	}

	// Generate markdown.
	docs := component.GenerateDocs(specs)

	// Output mode: --output file
	if cdOutputFile != "" {
		if err := os.WriteFile(cdOutputFile, []byte(docs), 0o644); err != nil {
			return fmt.Errorf("writing docs: %w", err)
		}
		fmt.Printf("  docs → %s\n", cdOutputFile)
		return nil
	}

	// Default: stdout.
	fmt.Print(docs)
	return nil
}

// commitReadme commits the updated README via the forge API.
func commitReadme(rootDir, readmePath, content string) error {
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

	branch := cdBranch

	if err := forgeClient.CommitFile(ctx, forge.CommitFileOptions{
		Branch:  branch,
		Path:    readmePath,
		Content: []byte(content),
		Message: "docs: update component input documentation",
	}); err != nil {
		return fmt.Errorf("committing README: %w", err)
	}

	fmt.Printf("  docs %s → %s on %s\n", colorGreen("✓"), readmePath, branch)
	return nil
}
