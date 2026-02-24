package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/sofmeright/stagefreight/src/build"
	"github.com/sofmeright/stagefreight/src/config"
	"github.com/sofmeright/stagefreight/src/gitver"
	"github.com/sofmeright/stagefreight/src/narrator"
	"github.com/sofmeright/stagefreight/src/registry"
)

var (
	ncFile             string
	ncSection          string
	ncPlain            bool
	ncInline           bool
	ncPlacementSection string
	ncPlacementMatch   string
	ncPlacementPos     string
	ncDryRun           bool
)

var narratorComposeCmd = &cobra.Command{
	Use:   "compose [items...]",
	Short: "Compose modules into a file section from the shell",
	Long: `Compose modules into a managed section of a markdown file.

Items are specified as type:value pairs with optional comma-separated fields:

  badge:<alt>,file:<path>,link:<url>
  shield:<path>,link:<url>,label:<text>
  text:<markdown content>
  break:

Examples:
  stagefreight narrator compose -f README.md -s badges \
    badge:release,file:.badges/release.svg,link:https://github.com/myorg/myrepo/releases \
    shield:docker/pulls/myorg/myrepo,link:https://hub.docker.com/r/myorg/myrepo

  stagefreight narrator compose -f README.md --plain \
    --placement-match "^## Installation" --placement-position above \
    text:"## Prerequisites"`,
	Args: cobra.MinimumNArgs(1),
	RunE: runNarratorCompose,
}

func init() {
	narratorComposeCmd.Flags().StringVarP(&ncFile, "file", "f", "", "target file path (required)")
	narratorComposeCmd.Flags().StringVarP(&ncSection, "section", "s", "", "target section name")
	narratorComposeCmd.Flags().BoolVar(&ncPlain, "plain", false, "output without section markers")
	narratorComposeCmd.Flags().BoolVar(&ncInline, "inline", false, "insert inline (no newline padding)")
	narratorComposeCmd.Flags().StringVar(&ncPlacementSection, "placement-section", "", "anchor to a named section")
	narratorComposeCmd.Flags().StringVar(&ncPlacementMatch, "placement-match", "", "anchor to a regex match")
	narratorComposeCmd.Flags().StringVar(&ncPlacementPos, "placement-position", "below", "position: above, below (default), replace")
	narratorComposeCmd.Flags().BoolVar(&ncDryRun, "dry-run", false, "preview changes without writing")

	_ = narratorComposeCmd.MarkFlagRequired("file")

	narratorCmd.AddCommand(narratorComposeCmd)
}

func runNarratorCompose(cmd *cobra.Command, args []string) error {
	rootDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Detect version for template resolution.
	var vi *gitver.VersionInfo
	vi, err = build.DetectVersion(rootDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  warning: version detection failed: %v\n", err)
	}

	// Resolve URL bases from narrator config (if available).
	linkBase := strings.TrimRight(cfg.Narrator.LinkBase, "/")
	rawBase := cfg.Narrator.RawBase
	if rawBase == "" && linkBase != "" {
		rawBase = registry.DeriveRawBase(linkBase)
	}
	rawBase = strings.TrimRight(rawBase, "/")

	// Parse CLI items into NarratorItems.
	items, err := parseCLIItems(args)
	if err != nil {
		return err
	}

	// Build modules.
	modules := buildModules(items, linkBase, rawBase, vi)
	if len(modules) == 0 {
		return fmt.Errorf("no valid modules produced from arguments")
	}

	// Compose.
	composed := narrator.Compose(modules)

	// Read the target file.
	path := ncFile
	if !filepath.IsAbs(path) {
		path = filepath.Join(rootDir, path)
	}

	content := ""
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		if !os.IsNotExist(readErr) {
			return fmt.Errorf("reading %s: %w", ncFile, readErr)
		}
	} else {
		content = string(raw)
	}

	original := content
	position := config.NormalizePosition(ncPlacementPos)

	// Try to replace existing section first.
	if !ncPlain && ncSection != "" {
		if updated, found := registry.ReplaceSection(content, ncSection, composed); found {
			content = updated
		} else {
			// New section — wrap and place.
			var wrapped string
			if ncInline {
				wrapped = registry.WrapSectionInline(ncSection, composed)
			} else {
				wrapped = registry.WrapSection(ncSection, composed)
			}
			content = placeWrapped(content, wrapped, position, ncPlacementSection, ncPlacementMatch)
		}
	} else if ncPlain {
		content = registry.PlaceContent(content, ncPlacementSection, position, ncPlacementMatch, composed, ncInline, true)
	} else {
		// No section name, not plain — just place raw.
		content = registry.PlaceContent(content, ncPlacementSection, position, ncPlacementMatch, composed, ncInline, false)
	}

	if ncDryRun {
		if content != original {
			fmt.Printf("  narrator %s (changed)\n", ncFile)
			fmt.Println(content)
		} else {
			fmt.Printf("  narrator %s (unchanged)\n", ncFile)
		}
		return nil
	}

	if content == original {
		fmt.Printf("  narrator %s (unchanged)\n", ncFile)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", ncFile, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", ncFile, err)
	}
	fmt.Printf("  narrator %s (updated)\n", ncFile)
	return nil
}

// placeWrapped places already-wrapped content using placement anchors.
func placeWrapped(content, wrapped, position, sectionAnchor, matchPattern string) string {
	if sectionAnchor != "" || matchPattern != "" {
		return registry.PlaceContent(content, sectionAnchor, position, matchPattern, wrapped, false, false)
	}

	switch position {
	case "top":
		return wrapped + "\n" + content
	case "bottom":
		return content + "\n" + wrapped
	default:
		return wrapped + "\n\n" + content
	}
}

// parseCLIItems converts CLI arguments like "badge:release,file:.badges/release.svg,link:..."
// into NarratorItem config entries.
func parseCLIItems(args []string) ([]config.NarratorItem, error) {
	var items []config.NarratorItem

	for _, arg := range args {
		item, err := parseCLIItem(arg)
		if err != nil {
			return nil, fmt.Errorf("parsing item %q: %w", arg, err)
		}
		items = append(items, item)
	}

	return items, nil
}

// parseCLIItem parses a single CLI item argument.
// Format: type:value[,field:value,...]
// Examples:
//
//	badge:release,file:.badges/release.svg,link:https://example.com
//	shield:docker/pulls/myorg/myrepo,link:https://hub.docker.com
//	text:Hello World
//	break:
func parseCLIItem(arg string) (config.NarratorItem, error) {
	var item config.NarratorItem

	// Handle break specially.
	if arg == "break:" || arg == "break" {
		t := true
		item.Break = &t
		return item, nil
	}

	// Split into type:rest.
	colonIdx := strings.Index(arg, ":")
	if colonIdx < 0 {
		return item, fmt.Errorf("expected type:value format")
	}

	moduleType := arg[:colonIdx]
	rest := arg[colonIdx+1:]

	switch moduleType {
	case "text":
		// Text takes everything after "text:" as literal content.
		item.Text = rest
		return item, nil

	case "badge":
		// Parse comma-separated fields: badge:<alt>,file:<path>,link:<url>
		fields := splitFields(rest)
		if len(fields) == 0 {
			return item, fmt.Errorf("badge requires at least an alt value")
		}
		item.Badge = fields[0]
		for _, f := range fields[1:] {
			k, v := splitField(f)
			switch k {
			case "file":
				item.File = v
			case "url":
				item.URL = v
			case "link":
				item.Link = v
			}
		}
		return item, nil

	case "shield":
		// Parse: shield:<path>,link:<url>,label:<text>
		fields := splitFields(rest)
		if len(fields) == 0 {
			return item, fmt.Errorf("shield requires a path")
		}
		item.Shield = fields[0]
		for _, f := range fields[1:] {
			k, v := splitField(f)
			switch k {
			case "link":
				item.Link = v
			case "label":
				item.Label = v
			}
		}
		return item, nil

	default:
		return item, fmt.Errorf("unknown module type %q", moduleType)
	}
}

// splitFields splits on commas but respects field:value pairs that may contain
// colons (like URLs). A field boundary is a comma followed by a known field name
// and colon.
func splitFields(s string) []string {
	knownFields := []string{"file:", "url:", "link:", "label:"}

	var fields []string
	start := 0

	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			// Check if this comma precedes a known field.
			rest := s[i+1:]
			isFieldBoundary := false
			for _, kf := range knownFields {
				if strings.HasPrefix(rest, kf) {
					isFieldBoundary = true
					break
				}
			}
			if isFieldBoundary {
				fields = append(fields, s[start:i])
				start = i + 1
			}
		}
	}
	fields = append(fields, s[start:])
	return fields
}

// splitField splits "key:value" on the first colon.
func splitField(s string) (string, string) {
	idx := strings.Index(s, ":")
	if idx < 0 {
		return s, ""
	}
	return s[:idx], s[idx+1:]
}
