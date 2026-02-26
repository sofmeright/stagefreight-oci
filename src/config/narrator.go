package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// NarratorConfig holds the top-level narrator configuration.
type NarratorConfig struct {
	LinkBase string                `yaml:"link_base"` // base URL for resolving relative links
	RawBase  string                `yaml:"raw_base"`  // base URL for raw file access (auto-derived if empty)
	Badges   NarratorBadgeDefaults `yaml:"badges"`    // default badge generation settings
	Files    []NarratorFileConfig  `yaml:"files"`     // files to compose into
}

// NarratorBadgeDefaults holds default settings for badge generation
// within narrator items. Individual items can override these.
type NarratorBadgeDefaults struct {
	Font     string  `yaml:"font"`      // default built-in font name (default: "dejavu-sans")
	FontSize float64 `yaml:"font_size"` // default pixel size (default: 11)
	FontFile string  `yaml:"font_file"` // default custom font file path
}

// NarratorFileConfig defines narrator composition for a single file.
type NarratorFileConfig struct {
	ID       string            `yaml:"id"`       // stable identifier for future overlay/selection
	Path     string            `yaml:"path"`     // file path (relative to project root)
	Sections []NarratorSection `yaml:"sections"` // sections to compose
}

// NarratorSection defines a managed section with placement and items.
type NarratorSection struct {
	ID        string            `yaml:"id"`        // stable identifier for future overlay/selection
	Name      string            `yaml:"name"`      // section name (used in <!-- sf:<name> --> markers)
	Placement NarratorPlacement `yaml:"placement"` // where to place the section
	Inline    bool              `yaml:"inline"`    // if true, insert inline (no newline padding) on first creation
	Plain     bool              `yaml:"plain"`     // if true, output without <!-- sf: --> markers
	Items     []NarratorItem    `yaml:"items"`     // modules to compose
}

// NarratorPlacement controls where a section is placed in the document.
// Supports both shorthand string ("top", "bottom") and full object form.
type NarratorPlacement struct {
	Section  string `yaml:"section"`  // reference to another <!-- sf:<name> --> section
	Match    string `yaml:"match"`    // regex pattern against document content
	Position string `yaml:"position"` // above/below (default)/replace (with aliases)
}

// UnmarshalYAML implements custom unmarshaling for placement shorthand.
// Accepts either a string ("top", "bottom") or a full object.
func (p *NarratorPlacement) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		s := value.Value
		switch strings.ToLower(s) {
		case "top":
			p.Position = "top"
		case "bottom":
			p.Position = "bottom"
		default:
			return fmt.Errorf("unknown placement shorthand %q (expected \"top\" or \"bottom\")", s)
		}
		return nil
	}

	// Full object form.
	type plain NarratorPlacement
	return value.Decode((*plain)(p))
}

// NormalizedPosition returns the canonical position from aliases.
// Returns "below" if empty or unrecognized.
func (p NarratorPlacement) NormalizedPosition() string {
	return NormalizePosition(p.Position)
}

// NormalizePosition maps position aliases to canonical forms.
// Returns "below" for empty or unrecognized values.
func NormalizePosition(pos string) string {
	switch strings.ToLower(strings.TrimSpace(pos)) {
	case "above", "over", "up":
		return "above"
	case "below", "under", "down", "bottom", "beneath", "":
		return "below"
	case "replace", "fill", "inside", "target":
		return "replace"
	case "top":
		return "top"
	}
	return "below"
}

// NarratorItem defines a single composable item. Exactly one module field must be set.
type NarratorItem struct {
	// Stable ID for future overlay/profile/selection.
	ID string `yaml:"id"`

	// Module types (exactly one must be set)
	Badge     string `yaml:"badge"`     // badge alt text / name
	Shield    string `yaml:"shield"`    // shields.io path
	Text      string `yaml:"text"`      // literal markdown text
	Component string `yaml:"component"` // component spec file path — renders input docs
	Break     *bool  `yaml:"break"`     // forces a line break; only break:true counts, break:false is equivalent to unset

	// Display fields
	File  string `yaml:"file"`  // display reference (defaults to Output when not set)
	URL   string `yaml:"url"`   // absolute image URL (asset location)
	Link  string `yaml:"link"`  // click target (hyperlink destination)
	Label string `yaml:"label"` // override label (badge alt text, shield override)

	// Badge generation fields (when badge + output are set, SVG is generated)
	Output   string  `yaml:"output"`    // SVG generation output path
	Value    string  `yaml:"value"`     // right side text (supports templates)
	Color    string  `yaml:"color"`     // hex color or "auto"
	Font     string  `yaml:"font"`      // per-item font override (built-in name)
	FontSize float64 `yaml:"font_size"` // per-item font size override
	FontFile string  `yaml:"font_file"` // per-item custom font file override
}

// IsBreak returns true if this item is a break module.
func (n NarratorItem) IsBreak() bool {
	return n.Break != nil && *n.Break
}

// HasGeneration returns true if this badge item should trigger SVG generation.
// Requires badge + output. Optional generation fields (value/color/font) have defaults.
func (n NarratorItem) HasGeneration() bool {
	return n.Badge != "" && n.Output != ""
}

// DisplayFile returns the file reference for narrator display.
// Falls back to Output if File is not explicitly set.
func (n NarratorItem) DisplayFile() string {
	if n.File != "" {
		return n.File
	}
	return n.Output
}

// ToBadgeSpec extracts badge generation fields into a reusable BadgeSpec.
func (n NarratorItem) ToBadgeSpec() BadgeSpec {
	label := n.Label
	if label == "" {
		label = n.Badge
	}
	return BadgeSpec{
		Label:    label,
		Value:    n.Value,
		Color:    n.Color,
		Output:   n.Output,
		Font:     n.Font,
		FontSize: n.FontSize,
		FontFile: n.FontFile,
	}
}

// DefaultNarratorConfig returns an empty narrator config (disabled by default).
func DefaultNarratorConfig() NarratorConfig {
	return NarratorConfig{}
}
