# StageFreight — Narrator & Badges

Narrator is StageFreight's content composition and injection system. It
generates SVG badge assets, composes badge rows and text into markdown,
and injects them into managed `<!-- sf:<name> -->` sections in any
document — your local README, Docker Hub descriptions, or any file
referenced by config.

Three config blocks work together:

| Block | Purpose |
|-------|---------|
| `badges:` | Generate SVG badge files (the asset pipeline) |
| `docker.readme:` | Sync README to registries with badge injection |
| `narrator:` | General-purpose section composition for any file |

---

## Badge Generation (`badges:`)

Generates SVG badge files with embedded fonts. Every font — built-in or
custom — gets base64 `@font-face` embedded in the SVG for pixel-perfect
rendering everywhere.

```yaml
badges:
  # Global defaults (applied to all items unless overridden)
  font: dejavu-sans             # built-in font name
  font_size: 11                 # pixel size
  # font_file: ./Custom.ttf     # custom TTF/OTF (overrides font)

  items:
    - name: release
      label: release
      value: "{version}"        # template variables: {version}, {env:VAR}, etc.
      color: auto               # "auto" = status-driven, or hex "#4c1"
      output: .stagefreight/badges/release.svg

    - name: build
      label: build
      value: "{env:BUILD_STATUS}"
      color: auto
      font: monofur             # per-badge font override
      output: .stagefreight/badges/build.svg

    - name: license
      label: license
      value: "AGPL-3.0+"
      color: "#310937"
      font: monofur
      output: .stagefreight/badges/license.svg

    - name: updated
      label: updated
      value: "{env:BUILD_DATE}"
      color: "#236144"
      output: .stagefreight/badges/updated.svg
```

### Badge Item Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | _(required)_ | Unique identifier |
| `label` | string | _(required)_ | Left side text |
| `value` | string | _(required)_ | Right side text (supports templates) |
| `color` | string | `"auto"` | Hex color or `"auto"` (status-driven) |
| `output` | string | `.stagefreight/badges/<name>.svg` | Output file path |
| `font` | string | from global | Per-badge font override |
| `font_size` | float | from global | Per-badge font size override |
| `font_file` | string | from global | Per-badge custom font file override |

### Built-In Fonts

`dejavu-sans` (default), `vera`, `monofur`, `vera-mono`, `ethereal`,
`playpen-sans`, `doto`, `penyae`

---

## Docker Hub README Sync (`docker.readme:`)

Syncs your README to container registries (Docker Hub, GHCR, Quay). Before
pushing, narrator composes badge entries into `<!-- sf:badges -->` sections,
rewrites relative links to absolute URLs, and applies regex transforms.

```yaml
docker:
  readme:
    # Explicitly enable/disable readme sync.
    # Default: auto-detected (enabled if any field is set).
    # enabled: true

    # Source README file (relative to project root)
    # Default: README.md
    # file: README.md

    # Short description for Docker Hub (max 100 chars)
    # Default: extracted from first paragraph of README
    description: "Declarative CI/CD CLI"

    # Base URL for resolving relative links in README
    # Relative paths like LICENSE become link_base/LICENSE
    link_base: "https://github.com/sofmeright/stagefreight/blob/main"

    # Base URL for raw file access (SVG images, etc.)
    # Auto-derived from link_base if empty:
    #   github.com/.../blob/main → raw.githubusercontent.com/.../main
    #   gitlab.com/.../-/blob/main → gitlab.com/.../-/raw/main
    #   gitea/.../src/branch/main → gitea/.../raw/branch/main
    # raw_base: ""

    # Legacy marker extraction (pre-narrator).
    # When true, only content between start/end markers is synced.
    # markers: false
    # start_marker: "<!-- dockerhub-start -->"
    # end_marker: "<!-- dockerhub-end -->"

    # ── Badge Injection ────────────────────────────────────
    # Narrator composes these into <!-- sf:badges --> sections.
    # Badges from committed SVGs use raw_base for the image URL.
    # Shields.io badges use url directly.
    badges:
      - alt: release
        file: ".stagefreight/badges/release.svg"
        link: "https://github.com/sofmeright/stagefreight/releases"

      - alt: build
        file: ".stagefreight/badges/build.svg"
        link: "https://github.com/sofmeright/stagefreight/actions"

      - alt: license
        file: ".stagefreight/badges/license.svg"
        link: LICENSE

      - alt: updated
        file: ".stagefreight/badges/updated.svg"

      - alt: pulls
        url: "https://img.shields.io/docker/pulls/prplanit/stagefreight"
        link: "https://hub.docker.com/r/prplanit/stagefreight"

    # Target a specific section instead of the default "badges"
    # - alt: docker-size
    #   url: "https://img.shields.io/docker/image-size/myorg/myrepo"
    #   section: docker-stats

    # ── Regex Transforms ───────────────────────────────────
    # Applied after badge injection and link rewriting.
    # transforms:
    #   - pattern: '!\[version\]\(.*?\)'
    #     replace: '![version](https://img.shields.io/badge/version-{version}-blue)'
```

### Badge Entry Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `alt` | string | _(required)_ | Image alt text |
| `file` | string | — | Relative path to committed SVG (resolved via `raw_base`) |
| `url` | string | — | Absolute image URL (shields.io, etc.) — mutually exclusive with `file` |
| `link` | string | — | Click target (absolute URL or relative path resolved via `link_base`) |
| `section` | string | `"badges"` | Target `<!-- sf:<section> -->` section name |

### How Badge Injection Works

1. Badge entries are grouped by `section` (default: `"badges"`)
2. Each group is composed through the narrator into a single markdown row
3. The result replaces the content of the `<!-- sf:<section> -->` markers
4. If markers don't exist, the section is inserted at the top of the document

This is the same `ReplaceSection()` / `WrapSection()` infrastructure that
all narrator-managed content uses — badges are just one module type.

---

## Narrator Composition (`narrator:`)

General-purpose section composition for any file. Composes modules (badges,
shields, text) into managed sections with configurable placement.

```yaml
narrator:
  # Base URLs for resolving relative paths in modules
  link_base: "https://github.com/myorg/myrepo/blob/main"
  # raw_base: auto-derived from link_base

  files:
    - path: README.md
      sections:

        # ── Badge row at the top ───────────────────────────
        - name: badges
          placement: top
          items:
            - badge: release
              file: ".stagefreight/badges/release.svg"
              link: "https://github.com/myorg/myrepo/releases"
            - badge: license
              file: ".stagefreight/badges/license.svg"
              link: LICENSE
            - shield: docker/pulls/prplanit/stagefreight
              link: "https://hub.docker.com/r/prplanit/stagefreight"

        # ── Multi-row with breaks ──────────────────────────
        - name: status
          placement:
            match: "^## Status"
            position: below
          items:
            - shield: github/actions/workflow/status/myorg/myrepo/ci.yml
              label: build
            - shield: docker/image-size/myorg/myrepo
            - break:
            - text: "**Version:** {version}"

        # ── Inline section (no newline padding) ────────────
        - name: api-version
          inline: true
          items:
            - text: "{version}"

        # ── Plain text (no markers) ────────────────────────
        - name: title
          plain: true
          placement: top
          items:
            - text: "# My Project"
            - break:
            - text: "A brief description of what this does."

        # ── Scoped replacement within a section ────────────
        - name: pulls-count
          placement:
            section: docker-stats
            match: "^Pulls: .*"
            position: replace
          items:
            - text: "Pulls: 12,847"
```

---

## Managed Sections

Content is injected into `<!-- sf:<name> -->` / `<!-- /sf:<name> -->` markers.
Everything between markers is replaced on each run. Everything outside is
never touched.

```markdown
<!-- sf:badges -->
[![release](https://...)](https://...) [![license](https://...)](https://...)
<!-- /sf:badges -->
```

### Block vs Inline

Detected automatically from marker placement in the source document.

**Block** — markers on separate lines, content gets newline padding:
```markdown
<!-- sf:badges -->
[![release](https://...)](https://...)
<!-- /sf:badges -->
```

**Inline** — markers on the same line, content inserted directly:
```markdown
| Version | <!-- sf:api-version -->1.2.3<!-- /sf:api-version --> |
```

Set `inline: true` in config for first-time creation of inline sections.

### Nested Sections

Sections can contain other sections. Target inner sections directly.

```markdown
<!-- sf:status-row -->
<!-- sf:build-badge -->[![build](...)](#)<!-- /sf:build-badge --> <!-- sf:coverage-badge -->[![cov](...)](#)<!-- /sf:coverage-badge -->
<!-- /sf:status-row -->
```

---

## Modules

Pluggable content producers. Each renders to inline markdown.

### Badge Module

```yaml
- badge: release                    # alt text
  file: ".stagefreight/badges/release.svg"  # relative path (resolved via raw_base)
  link: "https://..."               # click target
```

Produces: `[![release](https://raw.../release.svg)](https://...)`

### Shield Module

Shorthand for shields.io badges. Path appended to `https://img.shields.io/`.

```yaml
- shield: docker/pulls/myorg/myrepo
  label: pulls                      # override alt text (default: last path segment)
  link: "https://hub.docker.com/..."
```

Produces: `[![pulls](https://img.shields.io/docker/pulls/myorg/myrepo)](https://...)`

### Text Module

Literal markdown. Supports template variables.

```yaml
- text: "**Current version:** {version}"
```

### Break Module

Forces a line break. Items before the break are space-joined on one line,
items after start a new line.

```yaml
- break:
```

---

## Placement

Controls where sections are placed in the document.

### Shorthand

```yaml
placement: top          # start of document
placement: bottom       # end of document
```

### Full Form

```yaml
# Relative to another section
placement:
  section: badges
  position: below       # above | below (default) | replace

# Relative to a regex match
placement:
  match: "^## Installation"
  position: above

# Scoped: regex within a section
placement:
  section: docker-stats
  match: "^Pulls: .*"
  position: replace
```

### Position Aliases

| Behavior | Aliases |
|----------|---------|
| **above** | `above`, `over`, `up`, `top` |
| **below** (default) | `below`, `under`, `down`, `bottom`, `beneath` |
| **replace** | `replace`, `fill`, `inside`, `target` |

### Placement Matrix

| Anchor | `above` | `below` (default) | `replace` |
|--------|---------|-------------------|-----------|
| `section: X` | Before `<!-- sf:X -->` | After `<!-- /sf:X -->` | Replace content between markers |
| `match: regex` | Before matched line | After matched line | Replace matched line(s) |
| `section` + `match` | Above match within section | Below match within section | Replace match within section |
| `top` | — | First in document | — |
| `bottom` | — | Last in document | — |

---

## URL Resolution

| Field | Resolved via | Purpose |
|-------|-------------|---------|
| `file` | `raw_base` | Image source — needs raw file access |
| `link` (relative) | `link_base` | Click target — rendered page URL |
| `url` | used as-is | Absolute image URL |
| `link` (absolute) | used as-is | Absolute click target |

### `raw_base` Auto-Derivation

If `raw_base` is not set, it is derived from `link_base`:

| Forge | `link_base` | Derived `raw_base` |
|-------|-------------|---------------------|
| GitHub | `github.com/{owner}/{repo}/blob/{branch}` | `raw.githubusercontent.com/{owner}/{repo}/{branch}` |
| GitLab | `gitlab.com/{owner}/{repo}/-/blob/{branch}` | `gitlab.com/{owner}/{repo}/-/raw/{branch}` |
| Gitea | `{host}/{owner}/{repo}/src/branch/{branch}` | `{host}/{owner}/{repo}/raw/branch/{branch}` |

---

## Template Variables

All narrator modules support template variables across every field.

| Template | Description |
|----------|-------------|
| `{version}` | Full semantic version (e.g., `1.2.3`) |
| `{major}`, `{minor}`, `{patch}` | Semver components |
| `{major}.{minor}` | Composable (any combination) |
| `{sha}`, `{sha:N}` | Commit SHA (default 7, or N chars) |
| `{branch}` | Current branch name |
| `{env:VAR}` | Environment variable value |
| `{date}`, `{datetime}`, `{timestamp}` | UTC date formats |
| `{date:FORMAT}` | Custom Go time layout |
| `{commit.date}` | HEAD commit date |
| `{project.name}` | Repo name from git remote |
| `{project.url}` | Repo URL (SSH to HTTPS conversion) |
| `{project.license}` | SPDX identifier from LICENSE file |
| `{project.description}` | From config `docker.readme.description` |
| `{project.language}` | Auto-detected from lockfiles |
| `{docker.pulls}`, `{docker.pulls:raw}` | Docker Hub pull count (formatted / raw) |
| `{docker.stars}` | Docker Hub star count |
| `{docker.size}`, `{docker.size:raw}` | Image size (formatted / bytes) |
| `{docker.latest}` | Latest tag digest (12 chars) |

---

## CLI Commands

### `narrator compose` — Ad-Hoc Shell Mode

```bash
# Compose badges into a section
stagefreight narrator compose -f README.md -s badges \
  badge:release,file:.stagefreight/badges/release.svg,link:https://github.com/myorg/myrepo/releases \
  badge:license,file:.stagefreight/badges/license.svg,link:LICENSE \
  break: \
  shield:docker/pulls/myorg/myrepo,link:https://hub.docker.com/r/myorg/myrepo

# Insert plain text above a heading
stagefreight narrator compose -f README.md --plain \
  --placement-match "^## Installation" --placement-position above \
  text:"## Prerequisites" \
  break: \
  text:"Make sure you have Docker installed."
```

### `narrator run` — Config-Driven

```bash
# Process all narrator sections from .stagefreight.yml
stagefreight narrator run

# Dry-run to preview changes
stagefreight narrator run --dry-run
```

---

## Pipeline Integration

During `stagefreight docker build`, narrator runs automatically:

1. **Badge generation** — renders all `badges.items` to SVG files
2. **Docker README sync** — composes `docker.readme.badges` into
   `<!-- sf:badges -->` sections, rewrites links, applies transforms,
   pushes to each registry
3. **Build output** — badge section in pipeline output shows each
   generated badge with name, output path, font, size, and color
