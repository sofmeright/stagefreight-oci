package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/sofmeright/stagefreight/src/build"
	"github.com/sofmeright/stagefreight/src/component"
	"github.com/sofmeright/stagefreight/src/config"
	"github.com/sofmeright/stagefreight/src/gitver"
	"github.com/sofmeright/stagefreight/src/narrator"
	"github.com/sofmeright/stagefreight/src/registry"
)

var (
	nrDryRun bool
)

var narratorRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run narrator sections from config",
	Long: `Execute all narrator sections defined in the narrator config.

Each section is composed from its items and placed into the target file
according to its placement rules. Existing managed sections are replaced
idempotently; new sections are inserted at the specified position.`,
	RunE: runNarratorRun,
}

func init() {
	narratorRunCmd.Flags().BoolVar(&nrDryRun, "dry-run", false, "preview changes without writing files")

	narratorCmd.AddCommand(narratorRunCmd)
}

func runNarratorRun(cmd *cobra.Command, args []string) error {
	ncfg := cfg.Git.Narrator
	if len(ncfg.Files) == 0 {
		return fmt.Errorf("no narrator files configured in narrator.files")
	}

	rootDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Detect version for template resolution.
	var versionInfo *gitver.VersionInfo
	versionInfo, err = build.DetectVersion(rootDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  warning: version detection failed: %v\n", err)
	}

	// Resolve URL bases.
	linkBase := strings.TrimRight(ncfg.LinkBase, "/")
	rawBase := ncfg.RawBase
	if rawBase == "" && linkBase != "" {
		rawBase = registry.DeriveRawBase(linkBase)
	}
	rawBase = strings.TrimRight(rawBase, "/")

	for _, fileCfg := range ncfg.Files {
		if err := processNarratorFile(fileCfg, rootDir, linkBase, rawBase, versionInfo); err != nil {
			return err
		}
	}

	return nil
}

func processNarratorFile(fileCfg config.NarratorFileConfig, rootDir, linkBase, rawBase string, vi *gitver.VersionInfo) error {
	path := fileCfg.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(rootDir, path)
	}

	// Read existing file (or start empty).
	content := ""
	raw, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("narrator: reading %s: %w", fileCfg.Path, err)
		}
		// File doesn't exist yet — start fresh.
	} else {
		content = string(raw)
	}

	original := content

	for _, section := range fileCfg.Sections {
		content = applyNarratorSection(content, section, linkBase, rawBase, vi)
	}

	if nrDryRun {
		if content != original {
			fmt.Printf("  narrator %s (changed)\n", fileCfg.Path)
			fmt.Println(content)
		} else {
			fmt.Printf("  narrator %s (unchanged)\n", fileCfg.Path)
		}
		return nil
	}

	if content == original {
		fmt.Printf("  narrator %s (unchanged)\n", fileCfg.Path)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("narrator: creating directory for %s: %w", fileCfg.Path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("narrator: writing %s: %w", fileCfg.Path, err)
	}
	fmt.Printf("  narrator %s (updated)\n", fileCfg.Path)
	return nil
}

func applyNarratorSection(content string, section config.NarratorSection, linkBase, rawBase string, vi *gitver.VersionInfo) string {
	// Build modules from items.
	modules := buildModules(section.Items, linkBase, rawBase, vi)
	if len(modules) == 0 {
		return content
	}

	// Compose modules into content.
	composed := narrator.Compose(modules)
	if composed == "" {
		return content
	}

	// Resolve placement.
	position := section.Placement.NormalizedPosition()

	// If this is a named section (not plain), try to replace existing markers first.
	if !section.Plain && section.Name != "" {
		if updated, found := registry.ReplaceSection(content, section.Name, composed); found {
			return updated
		}
	}

	// Section doesn't exist yet (or plain mode) — use placement to insert.
	if section.Plain {
		return registry.PlaceContent(content, section.Placement.Section, position, section.Placement.Match, composed, section.Inline, true)
	}

	// Wrap in markers before placing.
	var wrapped string
	if section.Inline {
		wrapped = registry.WrapSectionInline(section.Name, composed)
	} else {
		wrapped = registry.WrapSection(section.Name, composed)
	}

	// Place the wrapped section.
	sectionAnchor := section.Placement.Section
	matchPattern := section.Placement.Match

	if sectionAnchor != "" || matchPattern != "" {
		return registry.PlaceContent(content, sectionAnchor, position, matchPattern, wrapped, section.Inline, false)
	}

	// No anchor — use document-level position.
	switch position {
	case "top":
		return wrapped + "\n" + content
	case "bottom":
		return content + "\n" + wrapped
	default:
		// Default: prepend to top.
		return wrapped + "\n\n" + content
	}
}

// buildModules converts NarratorItem config entries into narrator.Module instances.
func buildModules(items []config.NarratorItem, linkBase, rawBase string, vi *gitver.VersionInfo) []narrator.Module {
	var modules []narrator.Module

	for _, item := range items {
		switch {
		case item.IsBreak():
			modules = append(modules, narrator.BreakModule{})

		case item.Badge != "":
			mod := resolveBadgeItem(item, linkBase, rawBase)
			if mod != nil {
				modules = append(modules, mod)
			}

		case item.Shield != "":
			label := item.Label
			if label == "" {
				label = item.Shield
			}
			if vi != nil {
				label = gitver.ResolveTemplate(label, vi)
			}
			modules = append(modules, narrator.ShieldModule{
				Path:  item.Shield,
				Label: label,
				Link:  resolveLink(item.Link, linkBase),
			})

		case item.Text != "":
			text := item.Text
			if vi != nil {
				text = gitver.ResolveTemplate(text, vi)
			}
			modules = append(modules, narrator.TextModule{Text: text})

		case item.Component != "":
			spec, err := component.ParseSpec(item.Component)
			if err != nil {
				fmt.Fprintf(os.Stderr, "narrator: component %s: %v\n", item.Component, err)
				continue
			}
			docs := component.GenerateDocs([]*component.SpecFile{spec})
			modules = append(modules, narrator.ComponentModule{Docs: strings.TrimSpace(docs)})
		}
	}

	return modules
}

// resolveBadgeItem resolves a badge NarratorItem to a BadgeModule.
func resolveBadgeItem(item config.NarratorItem, linkBase, rawBase string) narrator.Module {
	var imgURL string
	if item.URL != "" {
		imgURL = item.URL
	} else if item.DisplayFile() != "" && rawBase != "" {
		imgURL = rawBase + "/" + strings.TrimPrefix(item.DisplayFile(), "./")
	} else {
		return nil
	}

	return narrator.BadgeModule{
		Alt:    item.Badge,
		ImgURL: imgURL,
		Link:   resolveLink(item.Link, linkBase),
	}
}

// resolveLink resolves a relative link against a base URL.
func resolveLink(link, linkBase string) string {
	if link == "" {
		return ""
	}
	if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") || strings.HasPrefix(link, "/") {
		return link
	}
	if linkBase != "" {
		return linkBase + "/" + strings.TrimPrefix(link, "./")
	}
	return link
}
