package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// NarratorConfig holds the top-level narrator configuration.
type NarratorConfig struct {
	LinkBase string               `yaml:"link_base"` // base URL for resolving relative links
	RawBase  string               `yaml:"raw_base"`  // base URL for raw file access (auto-derived if empty)
	Files    []NarratorFileConfig `yaml:"files"`      // files to compose into
}

// NarratorFileConfig defines narrator composition for a single file.
type NarratorFileConfig struct {
	Path     string              `yaml:"path"`     // file path (relative to project root)
	Sections []NarratorSection   `yaml:"sections"` // sections to compose
}

// NarratorSection defines a managed section with placement and items.
type NarratorSection struct {
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
	// Module types (exactly one must be set)
	Badge  string `yaml:"badge"`  // badge alt text / name
	Shield string `yaml:"shield"` // shields.io path
	Text   string `yaml:"text"`   // literal markdown text
	Break  *bool  `yaml:"break"`  // forces a line break (value ignored, just needs to be present)

	// Badge/Shield fields
	File  string `yaml:"file"`  // relative path to committed SVG (resolved via raw_base)
	URL   string `yaml:"url"`   // absolute image URL
	Link  string `yaml:"link"`  // click target
	Label string `yaml:"label"` // override label (shield module)
}

// IsBreak returns true if this item is a break module.
func (n NarratorItem) IsBreak() bool {
	return n.Break != nil
}

// DefaultNarratorConfig returns an empty narrator config (disabled by default).
func DefaultNarratorConfig() NarratorConfig {
	return NarratorConfig{}
}
