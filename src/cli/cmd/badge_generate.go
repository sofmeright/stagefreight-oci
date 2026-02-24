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
	Long: `Generate SVG badges defined in the badges config section.

Config-driven (no flags): generates all configured badge items, or named items if specified.
Ad-hoc (--label + --value): generates a single badge from flags.`,
	RunE: runBadgeGenerate,
}

func init() {
	badgeGenerateCmd.Flags().StringVar(&bgLabel, "label", "", "ad-hoc badge label (left side)")
	badgeGenerateCmd.Flags().StringVar(&bgValue, "value", "", "ad-hoc badge value (right side)")
	badgeGenerateCmd.Flags().StringVar(&bgColor, "color", "#4c1", "ad-hoc badge color (hex)")
	badgeGenerateCmd.Flags().StringVar(&bgStatus, "status", "", "status-driven color: passed, warning, critical")
	badgeGenerateCmd.Flags().StringVar(&bgOutput, "output", ".badges/custom.svg", "output file path")

	badgeCmd.AddCommand(badgeGenerateCmd)
}

func runBadgeGenerate(cmd *cobra.Command, args []string) error {
	eng, err := buildBadgeEngine()
	if err != nil {
		return err
	}

	// Ad-hoc mode: --label and --value provided
	if bgLabel != "" && bgValue != "" {
		return generateAdHocBadge(eng)
	}

	// Config-driven mode
	return generateConfigBadges(eng, args)
}

func buildBadgeEngine() (*badge.Engine, error) {
	var metrics *badge.FontMetrics
	var err error

	bcfg := cfg.Badges
	size := bcfg.FontSize
	if size == 0 {
		size = 11
	}

	if bcfg.FontFile != "" {
		metrics, err = badge.LoadFontFile(bcfg.FontFile, size)
	} else {
		fontName := bcfg.Font
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

func generateConfigBadges(eng *badge.Engine, names []string) error {
	items := cfg.Badges.Items
	if len(items) == 0 {
		return fmt.Errorf("no badge items configured in badges.items")
	}

	// Filter to named items if specified
	if len(names) > 0 {
		nameSet := make(map[string]bool, len(names))
		for _, n := range names {
			nameSet[n] = true
		}
		var filtered []config.BadgeItemConfig
		for _, item := range items {
			if nameSet[item.Name] {
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
		// Resolve per-item engine if font is overridden.
		itemEng := eng
		if item.Font != "" || item.FontFile != "" || item.FontSize != 0 {
			override, err := buildItemEngine(item, cfg.Badges)
			if err != nil {
				return fmt.Errorf("loading font for badge %s: %w", item.Name, err)
			}
			itemEng = override
		}

		// Resolve value templates
		value := item.Value
		if versionInfo != nil && value != "" {
			value = gitver.ResolveTemplateWithDir(value, versionInfo, rootDir)
		}
		value = gitver.ResolveDockerTemplates(value, dockerInfo)

		// Resolve color
		color := item.Color
		if color == "" || color == "auto" {
			color = badge.StatusColor(bgStatus)
		}

		// Resolve output path
		output := item.Output
		if output == "" {
			output = fmt.Sprintf(".badges/%s.svg", item.Name)
		}

		svg := itemEng.Generate(badge.Badge{
			Label: item.Label,
			Value: value,
			Color: color,
		})

		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			return fmt.Errorf("creating badge directory for %s: %w", item.Name, err)
		}
		if err := os.WriteFile(output, []byte(svg), 0o644); err != nil {
			return fmt.Errorf("writing badge %s: %w", item.Name, err)
		}
		fmt.Printf("  badge %s → %s\n", item.Name, output)
	}

	return nil
}

// buildItemEngine creates a badge engine for an item with font overrides.
// Falls back to global config values for any field not overridden.
func buildItemEngine(item config.BadgeItemConfig, global config.BadgesConfig) (*badge.Engine, error) {
	size := item.FontSize
	if size == 0 {
		size = global.FontSize
	}
	if size == 0 {
		size = 11
	}

	var metrics *badge.FontMetrics
	var err error

	switch {
	case item.FontFile != "":
		metrics, err = badge.LoadFontFile(item.FontFile, size)
	case item.Font != "":
		metrics, err = badge.LoadBuiltinFont(item.Font, size)
	case global.FontFile != "":
		metrics, err = badge.LoadFontFile(global.FontFile, size)
	default:
		fontName := global.Font
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
