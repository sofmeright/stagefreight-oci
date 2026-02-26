package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/sofmeright/stagefreight/src/badge"
	"github.com/sofmeright/stagefreight/src/build"
	"github.com/sofmeright/stagefreight/src/config"
	"github.com/sofmeright/stagefreight/src/gitver"
)

var (
	bgLabel  string
	bgValue  string
	bgColor  string
	bgStatus string
	bgOutput string
)

var badgeGenerateCmd = &cobra.Command{
	Use:   "generate [name...]",
	Short: "Generate SVG badges from config or flags",
	Long: `Generate SVG badges defined in narrator config items.

Config-driven (no flags): generates all narrator badge items with output paths, or named items if specified.
Ad-hoc (--label + --value): generates a single badge from flags.`,
	RunE: runBadgeGenerate,
}

func init() {
	badgeGenerateCmd.Flags().StringVar(&bgLabel, "label", "", "ad-hoc badge label (left side)")
	badgeGenerateCmd.Flags().StringVar(&bgValue, "value", "", "ad-hoc badge value (right side)")
	badgeGenerateCmd.Flags().StringVar(&bgColor, "color", "#4c1", "ad-hoc badge color (hex)")
	badgeGenerateCmd.Flags().StringVar(&bgStatus, "status", "", "status-driven color: passed, warning, critical")
	badgeGenerateCmd.Flags().StringVar(&bgOutput, "output", ".stagefreight/badges/custom.svg", "output file path")

	badgeCmd.AddCommand(badgeGenerateCmd)
}

func runBadgeGenerate(cmd *cobra.Command, args []string) error {
	defaults := cfg.Git.Narrator.Badges
	eng, err := buildBadgeEngine(defaults)
	if err != nil {
		return err
	}

	// Ad-hoc mode: --label and --value provided
	if bgLabel != "" && bgValue != "" {
		return generateAdHocBadge(eng)
	}

	// Config-driven mode
	return generateConfigBadges(eng, defaults, args)
}

func buildBadgeEngine(defaults config.NarratorBadgeDefaults) (*badge.Engine, error) {
	var metrics *badge.FontMetrics
	var err error

	size := defaults.FontSize
	if size == 0 {
		size = 11
	}

	if defaults.FontFile != "" {
		metrics, err = badge.LoadFontFile(defaults.FontFile, size)
	} else {
		fontName := defaults.Font
		if fontName == "" {
			fontName = "dejavu-sans"
		}
		metrics, err = badge.LoadBuiltinFont(fontName, size)
	}
	if err != nil {
		return nil, fmt.Errorf("loading badge font: %w", err)
	}

	return badge.New(metrics), nil
}

func generateAdHocBadge(eng *badge.Engine) error {
	color := bgColor
	if bgStatus != "" {
		color = badge.StatusColor(bgStatus)
	}

	svg := eng.Generate(badge.Badge{
		Label: bgLabel,
		Value: bgValue,
		Color: color,
	})

	if err := os.MkdirAll(filepath.Dir(bgOutput), 0o755); err != nil {
		return fmt.Errorf("creating badge directory: %w", err)
	}
	if err := os.WriteFile(bgOutput, []byte(svg), 0o644); err != nil {
		return fmt.Errorf("writing badge: %w", err)
	}
	fmt.Printf("  badge → %s\n", bgOutput)
	return nil
}

func generateConfigBadges(eng *badge.Engine, defaults config.NarratorBadgeDefaults, names []string) error {
	// Collect all narrator items that have generation capability
	var items []config.NarratorItem
	for _, f := range cfg.Git.Narrator.Files {
		for _, s := range f.Sections {
			for _, item := range s.Items {
				if item.HasGeneration() {
					items = append(items, item)
				}
			}
		}
	}

	if len(items) == 0 {
		return fmt.Errorf("no badge items with generation configured in git.narrator")
	}

	// Filter to named items if specified
	if len(names) > 0 {
		nameSet := make(map[string]bool, len(names))
		for _, n := range names {
			nameSet[n] = true
		}
		var filtered []config.NarratorItem
		for _, item := range items {
			// Match by badge name or ID
			if nameSet[item.Badge] || (item.ID != "" && nameSet[item.ID]) {
				filtered = append(filtered, item)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("no matching badge items for: %v", names)
		}
		items = filtered
	}

	rootDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Detect version for template resolution
	var versionInfo *gitver.VersionInfo
	versionInfo, err = build.DetectVersion(rootDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  warning: version detection failed: %v\n", err)
	}

	// Inject project description from config
	if cfg.Docker.Readme.Description != "" {
		gitver.SetProjectDescription(cfg.Docker.Readme.Description)
	}

	// Lazy Docker Hub info — only fetch if any badge value uses {docker.*}
	var dockerInfo *gitver.DockerHubInfo
	for _, item := range items {
		if strings.Contains(item.Value, "{docker.") {
			ns, repo := dockerHubFromConfig()
			if ns != "" && repo != "" {
				dockerInfo, _ = gitver.FetchDockerHubInfo(ns, repo)
			}
			break
		}
	}

	for _, item := range items {
		spec := item.ToBadgeSpec()

		// Resolve per-item engine if font is overridden.
		itemEng := eng
		if spec.Font != "" || spec.FontFile != "" || spec.FontSize != 0 {
			override, err := buildItemEngine(spec, defaults)
			if err != nil {
				return fmt.Errorf("loading font for badge %s: %w", item.Badge, err)
			}
			itemEng = override
		}

		// Resolve value templates
		value := spec.Value
		if versionInfo != nil && value != "" {
			value = gitver.ResolveTemplateWithDir(value, versionInfo, rootDir)
		}
		value = gitver.ResolveDockerTemplates(value, dockerInfo)

		// Resolve color
		color := spec.Color
		if color == "" || color == "auto" {
			color = badge.StatusColor(bgStatus)
		}

		svg := itemEng.Generate(badge.Badge{
			Label: spec.Label,
			Value: value,
			Color: color,
		})

		if err := os.MkdirAll(filepath.Dir(spec.Output), 0o755); err != nil {
			return fmt.Errorf("creating badge directory for %s: %w", item.Badge, err)
		}
		if err := os.WriteFile(spec.Output, []byte(svg), 0o644); err != nil {
			return fmt.Errorf("writing badge %s: %w", item.Badge, err)
		}
		fmt.Printf("  badge %s → %s\n", item.Badge, spec.Output)
	}

	return nil
}

// buildItemEngine creates a badge engine for a BadgeSpec with font overrides.
// Falls back to narrator badge defaults for any field not overridden.
func buildItemEngine(spec config.BadgeSpec, defaults config.NarratorBadgeDefaults) (*badge.Engine, error) {
	size := spec.FontSize
	if size == 0 {
		size = defaults.FontSize
	}
	if size == 0 {
		size = 11
	}

	var metrics *badge.FontMetrics
	var err error

	switch {
	case spec.FontFile != "":
		metrics, err = badge.LoadFontFile(spec.FontFile, size)
	case spec.Font != "":
		metrics, err = badge.LoadBuiltinFont(spec.Font, size)
	case defaults.FontFile != "":
		metrics, err = badge.LoadFontFile(defaults.FontFile, size)
	default:
		fontName := defaults.Font
		if fontName == "" {
			fontName = "dejavu-sans"
		}
		metrics, err = badge.LoadBuiltinFont(fontName, size)
	}
	if err != nil {
		return nil, err
	}

	return badge.New(metrics), nil
}
