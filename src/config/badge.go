package config

// BadgesConfig holds badge generation configuration.
type BadgesConfig struct {
	Font     string            `yaml:"font"`      // built-in font name (default: "dejavu-sans")
	FontSize float64           `yaml:"font_size"`  // pixel size (default: 11)
	FontFile string            `yaml:"font_file"`  // path to custom TTF/OTF (overrides Font)
	Items    []BadgeItemConfig `yaml:"items"`
}

// BadgeItemConfig defines a single badge to generate.
type BadgeItemConfig struct {
	Name     string  `yaml:"name"`      // unique identifier
	Label    string  `yaml:"label"`     // left side text
	Value    string  `yaml:"value"`     // right side text (supports {version} etc. templates)
	Color    string  `yaml:"color"`     // hex color or "auto" (status-driven, default)
	Output   string  `yaml:"output"`    // file path (default: .stagefreight/badges/<name>.svg)
	Font     string  `yaml:"font"`      // per-badge font override (built-in name)
	FontSize float64 `yaml:"font_size"` // per-badge font size override
	FontFile string  `yaml:"font_file"` // per-badge custom font file override
}

// DefaultBadgesConfig returns sensible defaults for badge generation.
func DefaultBadgesConfig() BadgesConfig {
	return BadgesConfig{
		Font:     "dejavu-sans",
		FontSize: 11,
	}
}
