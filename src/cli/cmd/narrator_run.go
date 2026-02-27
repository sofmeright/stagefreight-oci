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
	Short: "Run narrator items from config",
	Long: `Execute all narrator items defined in the narrator config.

Each item is composed from its kind and placed into the target file
according to its placement markers. Existing managed content between
markers is replaced idempotently.

Items sharing the same placement markers are composed together:
inline items are joined with spaces, block items with newlines.`,
	RunE: runNarratorRun,
}

func init() {
	narratorRunCmd.Flags().BoolVar(&nrDryRun, "dry-run", false, "preview changes without writing files")

	narratorCmd.AddCommand(narratorRunCmd)
}

func runNarratorRun(cmd *cobra.Command, args []string) error {
	if len(cfg.Narrator) == 0 {
		return fmt.Errorf("no narrator files configured")
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

	for _, fileCfg := range cfg.Narrator {
		if err := processNarratorFile(fileCfg, rootDir, versionInfo); err != nil {
			return err
		}
	}

	return nil
}

// placementKey is the grouping key for items sharing the same placement.
// Items with identical placement markers, mode, and inline flag are composed together.
type placementKey struct {
	StartMarker string
	EndMarker   string
	Mode        string
	Inline      bool
}

// placementGroup holds items sharing the same placement.
type placementGroup struct {
	Key   placementKey
	Items []config.NarratorItem
}

func processNarratorFile(fileCfg config.NarratorFile, rootDir string, vi *gitver.VersionInfo) error {
	path := fileCfg.File
	if !filepath.IsAbs(path) {
		path = filepath.Join(rootDir, path)
	}

	// Resolve URL bases from per-file config.
	linkBase := strings.TrimRight(fileCfg.LinkBase, "/")
	rawBase := ""
	if linkBase != "" {
		rawBase = registry.DeriveRawBase(linkBase)
	}
	rawBase = strings.TrimRight(rawBase, "/")

	// Read existing file (or start empty).
	content := ""
	raw, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("narrator: reading %s: %w", fileCfg.File, err)
		}
		// File doesn't exist yet — start fresh.
	} else {
		content = string(raw)
	}

	original := content

	// Group items by placement (items sharing the same markers are composed together).
	groups := groupItemsByPlacement(fileCfg.Items)

	for _, group := range groups {
		// Build modules from items in this group.
		modules := buildModulesV2(group.Items, linkBase, rawBase, vi)
		if len(modules) == 0 {
			continue
		}

		// Compose modules: inline items joined with space, block items with newline.
		var composed string
		if group.Key.Inline {
			composed = narrator.ComposeInline(modules)
		} else {
			composed = narrator.Compose(modules)
		}
		if composed == "" {
			continue
		}

		// Replace content between the placement markers.
		if group.Key.StartMarker != "" && group.Key.EndMarker != "" {
			updated, found := registry.ReplaceBetween(content, group.Key.StartMarker, group.Key.EndMarker, composed)
			if found {
				content = updated
			} else if verbose {
				fmt.Fprintf(os.Stderr, "  warning: markers not found in %s: %s ... %s\n",
					fileCfg.File, group.Key.StartMarker, group.Key.EndMarker)
			}
		}
	}

	if nrDryRun {
		if content != original {
			fmt.Printf("  narrator %s (changed)\n", fileCfg.File)
			fmt.Println(content)
		} else {
			fmt.Printf("  narrator %s (unchanged)\n", fileCfg.File)
		}
		return nil
	}

	if content == original {
		fmt.Printf("  narrator %s (unchanged)\n", fileCfg.File)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("narrator: creating directory for %s: %w", fileCfg.File, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("narrator: writing %s: %w", fileCfg.File, err)
	}
	fmt.Printf("  narrator %s (updated)\n", fileCfg.File)
	return nil
}

// groupItemsByPlacement groups items by their placement key, preserving declaration order.
// Items with the same (between markers, mode, inline) are collected into one group.
func groupItemsByPlacement(items []config.NarratorItem) []placementGroup {
	var groups []placementGroup
	keyIndex := make(map[placementKey]int)

	for _, item := range items {
		key := placementKey{
			StartMarker: item.Placement.Between[0],
			EndMarker:   item.Placement.Between[1],
			Mode:        item.Placement.Mode,
			Inline:      item.Placement.Inline,
		}

		if idx, ok := keyIndex[key]; ok {
			groups[idx].Items = append(groups[idx].Items, item)
		} else {
			keyIndex[key] = len(groups)
			groups = append(groups, placementGroup{
				Key:   key,
				Items: []config.NarratorItem{item},
			})
		}
	}

	return groups
}

// buildModulesV2 converts v2 NarratorItem entries into narrator.Module instances.
// Dispatches on item.Kind instead of checking which field is set.
func buildModulesV2(items []config.NarratorItem, linkBase, rawBase string, vi *gitver.VersionInfo) []narrator.Module {
	var modules []narrator.Module

	for _, item := range items {
		switch item.Kind {
		case "break":
			modules = append(modules, narrator.BreakModule{})

		case "badge":
			mod := resolveBadgeItemV2(item, linkBase, rawBase)
			if mod != nil {
				modules = append(modules, mod)
			}

		case "shield":
			label := item.Text
			if label == "" {
				label = item.Shield
			}
			if vi != nil {
				label = gitver.ResolveTemplateWithDirAndVars(label, vi, "", cfg.Vars)
			}
			modules = append(modules, narrator.ShieldModule{
				Path:  item.Shield,
				Label: label,
				Link:  resolveLink(item.Link, linkBase),
			})

		case "text":
			text := item.Content
			if vi != nil {
				text = gitver.ResolveTemplateWithDirAndVars(text, vi, "", cfg.Vars)
			}
			modules = append(modules, narrator.TextModule{Text: text})

		case "component":
			spec, err := component.ParseSpec(item.Spec)
			if err != nil {
				fmt.Fprintf(os.Stderr, "narrator: component %s: %v\n", item.Spec, err)
				continue
			}
			docs := component.GenerateDocs([]*component.SpecFile{spec})
			modules = append(modules, narrator.ComponentModule{Docs: strings.TrimSpace(docs)})
		}
	}

	return modules
}

// resolveBadgeItemV2 resolves a v2 badge NarratorItem to a BadgeModule for markdown composition.
// Uses the badge's Output path (SVG file) with rawBase to construct the image URL.
func resolveBadgeItemV2(item config.NarratorItem, linkBase, rawBase string) narrator.Module {
	var imgURL string
	if item.Output != "" && rawBase != "" {
		imgURL = rawBase + "/" + strings.TrimPrefix(item.Output, "./")
	}

	if imgURL == "" {
		return nil
	}

	return narrator.BadgeModule{
		Alt:    item.Text,
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
