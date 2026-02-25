package config

// Level controls how much of the codebase gets scanned.
type Level string

const (
	LevelChanged Level = "changed"
	LevelFull    Level = "full"
)

// ModuleConfig holds per-module overrides.
type ModuleConfig struct {
	Enabled *bool          `yaml:"enabled,omitempty"`
	Options map[string]any `yaml:"options,omitempty"`
}

// LintConfig holds lint-specific configuration.
type LintConfig struct {
	Level         Level                   `yaml:"level"`
	CacheDir      string                  `yaml:"cache_dir"`
	TargetBranch  string                  `yaml:"target_branch"`
	Exclude       []string                `yaml:"exclude"`
	Modules       map[string]ModuleConfig `yaml:"modules"`
	LargeFilesMax int64                   `yaml:"large_files_max"`
}

// DefaultLintConfig returns production defaults.
func DefaultLintConfig() LintConfig {
	return LintConfig{
		Level:         LevelChanged,
		Exclude:       []string{},
		Modules:       map[string]ModuleConfig{},
		LargeFilesMax: 500 * 1024, // 500 KB
	}
}
