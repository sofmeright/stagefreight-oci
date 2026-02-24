package freshness

import (
	"strings"

	masterminds "github.com/Masterminds/semver/v3"
)

// VersionDelta describes how far behind a dependency is.
type VersionDelta struct {
	Major int
	Minor int
	Patch int
}

// IsZero returns true when there is no version difference.
func (d VersionDelta) IsZero() bool {
	return d.Major == 0 && d.Minor == 0 && d.Patch == 0
}

// decomposedTag holds the semver portion and any suffix of a container tag.
// Example: "1.25-alpine" → Version "1.25.0", Suffix "alpine"
//          "3.22.1"      → Version "3.22.1", Suffix ""
//          "noble"       → nil Version (non-versioned)
type decomposedTag struct {
	Version *masterminds.Version
	Suffix  string
	Raw     string
}

// decomposeTag splits a tag string into its semver version and suffix.
// Returns a non-nil Version when the tag starts with a parseable version.
func decomposeTag(tag string) decomposedTag {
	dt := decomposedTag{Raw: tag}

	// Strip leading 'v' if present (common in GitHub releases).
	clean := tag
	if strings.HasPrefix(clean, "v") {
		clean = clean[1:]
	}

	// Split on first hyphen to separate version from suffix.
	// "1.25-alpine" → ("1.25", "alpine")
	// "1.25.0"      → ("1.25.0", "")
	versionPart := clean
	if idx := strings.IndexByte(clean, '-'); idx >= 0 {
		versionPart = clean[:idx]
		dt.Suffix = clean[idx+1:]
	}

	// Attempt semver parse; if the tag is non-versioned (e.g. "noble") this
	// will fail and Version stays nil.
	v, err := masterminds.NewVersion(versionPart)
	if err == nil {
		dt.Version = v
	}

	return dt
}

// compareTags computes the delta between current and latest tags.
// Tags must share the same suffix family (both alpine, both empty, etc.).
// Returns zero delta if versions can't be compared.
func compareTags(current, latest string) VersionDelta {
	cur := decomposeTag(current)
	lat := decomposeTag(latest)

	if cur.Version == nil || lat.Version == nil {
		return VersionDelta{}
	}

	return VersionDelta{
		Major: int(lat.Version.Major()) - int(cur.Version.Major()),
		Minor: int(lat.Version.Minor()) - int(cur.Version.Minor()),
		Patch: int(lat.Version.Patch()) - int(cur.Version.Patch()),
	}
}

// compareVersionStrings compares two bare version strings (no tag suffix).
func compareVersionStrings(current, latest string) VersionDelta {
	cur := parseVersion(current)
	lat := parseVersion(latest)
	if cur == nil || lat == nil {
		return VersionDelta{}
	}
	return VersionDelta{
		Major: int(lat.Major()) - int(cur.Major()),
		Minor: int(lat.Minor()) - int(cur.Minor()),
		Patch: int(lat.Patch()) - int(cur.Patch()),
	}
}

// parseVersion attempts to parse a version string, stripping leading 'v'.
func parseVersion(s string) *masterminds.Version {
	clean := strings.TrimPrefix(s, "v")
	v, err := masterminds.NewVersion(clean)
	if err != nil {
		return nil
	}
	return v
}

// filterTagsBySuffix returns tags from the list that share the same suffix,
// excluding date-like tags (e.g. "20220328") that aren't real semver.
func filterTagsBySuffix(tags []string, suffix string) []decomposedTag {
	var out []decomposedTag
	for _, t := range tags {
		dt := decomposeTag(t)
		if dt.Version != nil && dt.Suffix == suffix && !isDateLikeVersion(dt.Version) {
			out = append(out, dt)
		}
	}
	return out
}

// isDateLikeVersion returns true if the version looks like a date (YYYYMMDD)
// rather than real semver. These show up in Docker Hub tags for Alpine, Ubuntu,
// etc. and would otherwise win any semver comparison (20220328.0.0 > 3.22.1).
func isDateLikeVersion(v *masterminds.Version) bool {
	// Date tags are single-component numbers >= 19700101 with no minor/patch.
	return v.Minor() == 0 && v.Patch() == 0 && v.Major() >= 19700101
}

// latestInFamily finds the highest version among decomposed tags.
func latestInFamily(tags []decomposedTag) *decomposedTag {
	if len(tags) == 0 {
		return nil
	}
	best := &tags[0]
	for i := 1; i < len(tags); i++ {
		if tags[i].Version.GreaterThan(best.Version) {
			best = &tags[i]
		}
	}
	return best
}
