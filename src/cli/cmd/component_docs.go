package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/sofmeright/stagefreight/src/component"
	"github.com/sofmeright/stagefreight/src/forge"
	"github.com/sofmeright/stagefreight/src/output"
	"github.com/sofmeright/stagefreight/src/registry"
)

var (
	cdSpecs      []string
	cdOutputFile string
	cdReadme     string
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
  - --readme: inject docs between markers in target file
  - --commit: inject docs and commit updated file via forge API`,
	RunE: runComponentDocs,
}

func init() {
	componentDocsCmd.Flags().StringSliceVar(&cdSpecs, "spec", nil, "component spec file(s) to parse (repeatable)")
	componentDocsCmd.Flags().StringVarP(&cdOutputFile, "output", "o", "", "write docs to file")
	componentDocsCmd.Flags().StringVar(&cdReadme, "readme", "", "inject docs between <!-- sf:<name> --> markers in target file (section name from git.narrator config)")
	componentDocsCmd.Flags().BoolVar(&cdCommit, "commit", false, "commit updated target file via forge API")
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

	if cdOutputFile != "" && (cdReadme != "" || cdCommit) {
		return fmt.Errorf("--output cannot be combined with --readme/--commit")
	}

	start := time.Now()

	// Parse all spec files.
	var specs []*component.SpecFile
	totalInputs := 0
	for _, path := range specFiles {
		spec, err := component.ParseSpec(path)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		specs = append(specs, spec)
		totalInputs += len(spec.Inputs)
		if verbose {
			fmt.Fprintf(os.Stderr, "  parsed %s: %d inputs\n", path, len(spec.Inputs))
		}
	}

	// Generate markdown.
	docs := component.GenerateDocs(specs)

	// Resolve target and section name once from narrator config.
	cfgTarget, cfgSection := resolveComponentTarget()

	// Resolve target file: --readme CLI flag → narrator config lookup
	target := cdReadme
	if target == "" && cdCommit {
		target = cfgTarget
	}

	// Output mode: --output file (raw write)
	if cdOutputFile != "" {
		if err := os.WriteFile(cdOutputFile, []byte(docs), 0o644); err != nil {
			return fmt.Errorf("writing docs: %w", err)
		}
		elapsed := time.Since(start)
		useColor := output.UseColor()
		w := os.Stdout
		sec := output.NewSection(w, "Component Docs", elapsed, useColor)
		sec.Row("%-16s%d inputs from %d spec(s)", "parsed", totalInputs, len(specs))
		output.RowStatus(sec, cdOutputFile, "", "success", useColor)
		sec.Close()
		return nil
	}

	// Output mode: --readme or --commit (marker injection)
	if target != "" {
		sectionName := cfgSection
		if sectionName == "" {
			return fmt.Errorf("cannot determine section name: no component item found in git.narrator config")
		}

		existing, err := os.ReadFile(target)
		if err != nil {
			return fmt.Errorf("reading target file %s: %w", target, err)
		}

		updated, found := registry.ReplaceSection(string(existing), sectionName, docs)
		if !found {
			return fmt.Errorf("section markers <!-- sf:%s --> not found in %s", sectionName, target)
		}

		if err := os.WriteFile(target, []byte(updated), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", target, err)
		}

		// Optionally commit via forge API
		if cdCommit {
			rootDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
			if err := commitReadme(rootDir, target, updated); err != nil {
				return err
			}
		}

		elapsed := time.Since(start)
		useColor := output.UseColor()
		w := os.Stdout
		sec := output.NewSection(w, "Component Docs", elapsed, useColor)
		sec.Row("%-16s%d inputs from %d spec(s)", "parsed", totalInputs, len(specs))
		status := "success"
		detail := "updated"
		if cdCommit {
			detail = "updated + committed"
		}
		output.RowStatus(sec, target, detail, status, useColor)
		sec.Close()
		return nil
	}

	// Default: stdout only (pipeable, no section frame).
	fmt.Print(docs)
	return nil
}

// resolveComponentTarget walks narrator config to find the file path and
// section name for the first component item. Returns ("", "") if not found.
func resolveComponentTarget() (filePath, sectionName string) {
	for _, f := range cfg.Git.Narrator.Files {
		for _, s := range f.Sections {
			for _, item := range s.Items {
				if item.Component != "" {
					return f.Path, s.Name
				}
			}
		}
	}
	return "", ""
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
