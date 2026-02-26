package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Validate checks structural invariants of a loaded Config.
// Returns warnings (deprecations, typo detection) and a hard error if
// the config is structurally invalid. Config package never prints —
// warnings are returned for the CLI to format.
func Validate(cfg *Config) (warnings []string, err error) {
	var errs []string

	// Validate policy keys
	for name := range cfg.Git.Policy.Tags {
		if !isIdentifier(name) {
			errs = append(errs, fmt.Sprintf("git.policy.tags: key %q is not a valid identifier (must match [a-zA-Z][a-zA-Z0-9_.\\-]*)", name))
		}
	}
	for name := range cfg.Git.Policy.Branches {
		if !isIdentifier(name) {
			errs = append(errs, fmt.Sprintf("git.policy.branches: key %q is not a valid identifier (must match [a-zA-Z][a-zA-Z0-9_.\\-]*)", name))
		}
	}

	// Validate narrator files
	fileIDs := make(map[string]bool)
	for fi, f := range cfg.Git.Narrator.Files {
		fpath := fmt.Sprintf("git.narrator.files[%d]", fi)

		// File ID uniqueness
		if f.ID != "" {
			if fileIDs[f.ID] {
				errs = append(errs, fmt.Sprintf("%s: duplicate file id %q", fpath, f.ID))
			}
			fileIDs[f.ID] = true
		}

		sectionIDs := make(map[string]bool)
		for si, s := range f.Sections {
			spath := fmt.Sprintf("%s.sections[%d]", fpath, si)

			// Section ID uniqueness (within file)
			if s.ID != "" {
				if sectionIDs[s.ID] {
					errs = append(errs, fmt.Sprintf("%s: duplicate section id %q", spath, s.ID))
				}
				sectionIDs[s.ID] = true
			}

			itemIDs := make(map[string]bool)
			for ii, item := range s.Items {
				ipath := fmt.Sprintf("%s.items[%d]", spath, ii)

				// Item ID uniqueness (within section)
				if item.ID != "" {
					if itemIDs[item.ID] {
						errs = append(errs, fmt.Sprintf("%s: duplicate item id %q", ipath, item.ID))
					}
					itemIDs[item.ID] = true
				}

				// Item kind validation + badge contract
				iwarns, ierrs := validateNarratorItem(item, ipath)
				warnings = append(warnings, iwarns...)
				errs = append(errs, ierrs...)
			}
		}
	}

	if len(errs) > 0 {
		return warnings, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return warnings, nil
}

// validateNarratorItem checks kind exclusivity, badge generation contract,
// and output path safety for a single narrator item.
func validateNarratorItem(item NarratorItem, path string) (warnings []string, errs []string) {
	// Count active kinds
	kindCount := 0
	var kindName string

	if item.Badge != "" {
		kindCount++
		kindName = "badge"
	}
	if item.Shield != "" {
		kindCount++
		kindName = "shield"
	}
	if item.Text != "" {
		kindCount++
		kindName = "text"
	}
	if item.Component != "" {
		kindCount++
		kindName = "component"
	}
	if item.Break != nil && *item.Break {
		kindCount++
		kindName = "break"
	}

	if kindCount == 0 {
		errs = append(errs, fmt.Sprintf("%s: no module kind set (need one of badge, shield, text, component, break)", path))
		return
	}
	if kindCount > 1 {
		errs = append(errs, fmt.Sprintf("%s: multiple module kinds set (exactly one required)", path))
		return
	}

	// Output requires badge kind
	if item.Output != "" && item.Badge == "" {
		errs = append(errs, fmt.Sprintf("%s: output requires badge kind", path))
		return
	}

	// Badge-specific validation
	if kindName == "badge" {
		hasGenFields := item.Value != "" || item.Color != "" || item.Font != "" ||
			item.FontSize != 0 || item.FontFile != ""

		if item.Output != "" {
			// Generation mode — validate output path
			if pathErrs := validateOutputPath(item.Output, path); len(pathErrs) > 0 {
				errs = append(errs, pathErrs...)
			}
		} else {
			// No output — display-only mode
			if hasGenFields {
				label := item.Badge
				if item.ID != "" {
					label = item.ID
				}
				// file→output deprecation alias: if file is set, treat as output
				if item.File != "" {
					warnings = append(warnings, fmt.Sprintf("%s: badge %q has file but no output; treating file as output (deprecated, use output instead)", path, label))
					// The actual migration happens in the caller after Validate
				} else {
					errs = append(errs, fmt.Sprintf("%s: badge %q has generation fields but no output path", path, label))
				}
			} else if item.File == "" && item.URL == "" {
				label := item.Badge
				if item.ID != "" {
					label = item.ID
				}
				errs = append(errs, fmt.Sprintf("%s: badge %q must have output for generation or file/url for display", path, label))
			}
		}
	}

	return
}

// validateOutputPath checks that an output path is safe.
func validateOutputPath(p string, itemPath string) []string {
	var errs []string

	if p == "" {
		errs = append(errs, fmt.Sprintf("%s: output path is empty", itemPath))
		return errs
	}

	// Absolute path
	if filepath.IsAbs(p) {
		errs = append(errs, fmt.Sprintf("%s: output path %q must be relative, not absolute", itemPath, p))
		return errs
	}

	// Tilde
	if strings.HasPrefix(p, "~") {
		errs = append(errs, fmt.Sprintf("%s: output path %q must not start with ~", itemPath, p))
		return errs
	}

	// Windows drive prefix
	if len(p) >= 2 && p[1] == ':' && ((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z')) {
		errs = append(errs, fmt.Sprintf("%s: output path %q looks like a Windows drive path", itemPath, p))
		return errs
	}

	// Path traversal
	if strings.Contains(p, "..") {
		errs = append(errs, fmt.Sprintf("%s: output path %q must not contain '..'", itemPath, p))
		return errs
	}

	// Normalize: strip leading ./ then compare with filepath.Clean
	normalized := strings.TrimPrefix(p, "./")
	clean := filepath.Clean(normalized)
	if clean != normalized {
		errs = append(errs, fmt.Sprintf("%s: output path %q is not in canonical form (cleaned to %q)", itemPath, p, clean))
		return errs
	}

	return errs
}

// applyDeprecationAliases modifies the config in-place to apply backward-compatible
// aliases (e.g., file→output for badge items). Returns warnings for each alias applied.
// Called by LoadWithWarnings after Validate.
func applyDeprecationAliases(cfg *Config) []string {
	var warnings []string

	for fi := range cfg.Git.Narrator.Files {
		for si := range cfg.Git.Narrator.Files[fi].Sections {
			for ii := range cfg.Git.Narrator.Files[fi].Sections[si].Items {
				item := &cfg.Git.Narrator.Files[fi].Sections[si].Items[ii]
				if item.Badge != "" && item.Output == "" && item.File != "" {
					hasGenFields := item.Value != "" || item.Color != "" || item.Font != "" ||
						item.FontSize != 0 || item.FontFile != ""
					if hasGenFields {
						item.Output = item.File
						label := item.Badge
						if item.ID != "" {
							label = item.ID
						}
						warnings = append(warnings, fmt.Sprintf("badge %q: file used as output (deprecated, use output field)", label))
					}
				}
			}
		}
	}

	return warnings
}
