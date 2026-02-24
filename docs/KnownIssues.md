# Known Issues

## Minor

### `color: auto` falls through to default grey for version strings

`StatusColor()` in `src/badge/engine.go` only maps status keywords (`passing`, `failed`, `warning`, etc.) to colors. When `color: auto` is used on a badge whose value is a version string (e.g., `v0.1.1`), none of the keywords match and it falls back to default grey.

**Workaround:** Use an explicit hex color instead of `auto` for version badges. The release badge currently uses `#74ecbe` (mint).

**Future:** `auto` could be version-aware — stable semver green, prerelease yellow, `0.x.x` teal, etc.
