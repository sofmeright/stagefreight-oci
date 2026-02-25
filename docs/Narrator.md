# Narrator — Managed README Sections

Narrator is StageFreight's system for composing and injecting generated content into markdown files. It uses HTML comment markers to define managed regions that are surgically replaced on each run, leaving everything else untouched.

## Managed Sections

A managed section is a region of a markdown file wrapped in `<!-- sf:<name> -->` markers:

```markdown
<!-- sf:badges -->
[![release](https://...)](https://...) [![license](https://...)](https://...)
<!-- /sf:badges -->
```

**Rules:**

- Everything between the markers is owned by StageFreight and will be replaced on each run.
- Everything outside the markers is never touched.
- Sections can appear anywhere in the document — top, middle, inside tables, inline in paragraphs.
- Section names are freeform. Use descriptive names: `badges`, `status-badges`, `api-table`, `docker-stats`.
- Runs are idempotent. Running the same config twice produces identical output.
- If markers don't exist in the document when a section is first composed, the wrapped section is inserted based on placement config (default: top of document).

## Block vs. Inline Sections

Sections operate in two modes, detected automatically from how the markers are placed in the source document.

### Block Sections

Markers on separate lines. Content gets newline padding for readability.

```markdown
<!-- sf:badges -->
[![release](https://...)](https://...) [![license](https://...)](https://...)
<!-- /sf:badges -->
```

### Inline Sections

Markers on the same line (or with no newlines between them). Content is inserted directly with no padding — enabling sections inside table cells, list items, or mid-paragraph.

```markdown
| Service | Status | Version |
|---------|--------|---------|
| API | <!-- sf:api-status -->healthy<!-- /sf:api-status --> | <!-- sf:api-version -->1.2.3<!-- /sf:api-version --> |
| DB  | <!-- sf:db-status -->degraded<!-- /sf:db-status --> | <!-- sf:db-version -->15.4<!-- /sf:db-version --> |
```

Each cell is independently managed. The table structure, pipes, and headers are static markdown you wrote. Only the content between markers changes.

Inline sections work anywhere markdown allows HTML comments:

```markdown
Current build: <!-- sf:build-status -->passing<!-- /sf:build-status --> | Coverage: <!-- sf:coverage -->87%<!-- /sf:coverage -->
```

When creating a new section via config, set `inline: true` to insert it inline (no newline padding) on first creation. After creation, block vs. inline mode is auto-detected from marker placement in the document.

### Nested Sections

Sections can contain other sections:

```markdown
<!-- sf:status-row -->
<!-- sf:build-badge -->[![build](...)](#)<!-- /sf:build-badge --> <!-- sf:coverage-badge -->[![coverage](...)](#)<!-- /sf:coverage-badge -->
<!-- /sf:status-row -->
```

Target inner sections directly. Replacing an outer section wholesale will replace inner markers too.

## Plain Mode

Set `plain: true` to output composed content without `<!-- sf: -->` markers. Content is inserted as plain text — no markers, no wrapping.

Plain mode is useful for:
- Shell-driven README generation where you control the entire document
- Composing content that doesn't need to be idempotently managed
- Generating content for tools that don't understand HTML comments

```yaml
narrator:
  files:
    - path: README.md
      sections:
        - name: intro
          plain: true
          placement: top
          items:
            - text: "# My Project"
            - break:
            - text: "A brief description of what this does."
```

## Placement

Sections support config-driven placement relative to other content in the document.

### Anchors

| Field | Description |
|-------|-------------|
| `section` | Reference to another `<!-- sf:<name> -->` section |
| `match` | Regex pattern against document content |

### Position

One field with generous aliases. **Default is `below`** — forgetting `position:` never puts something at the top by surprise.

| Behavior | Aliases | Description |
|----------|---------|-------------|
| **above** | `above`, `over`, `up`, `top` | Insert before the anchor |
| **below** | `below`, `under`, `down`, `bottom`, `beneath` | Insert after the anchor **(default)** |
| **replace** | `replace`, `fill`, `inside`, `target` | Replace the anchor content |

Special shorthands: `placement: top` (start of document), `placement: bottom` (end of document).

### Scoping

`section:` and `match:` can be combined — `section:` scopes the regex to only search within that section's content:

```yaml
# Replace a specific line inside a section
- name: pulls-count
  placement:
    section: docker-stats
    match: "^pulls: .*"
    position: replace
  items:
    - text: "pulls: 12,847"
```

### Full Placement Matrix

| Anchor | `above` | `below` (default) | `replace` |
|--------|---------|-------------------|-----------|
| `section: X` | Insert before `<!-- sf:X -->` | Insert after `<!-- /sf:X -->` | Replace content between markers |
| `match: regex` | Insert before matched line | Insert after matched line | Replace matched line(s) |
| `section` + `match` | Above match within section | Below match within section | Replace match within section |
| `top` | — | First in document | — |
| `bottom` | — | Last in document | — |

### Placement Config Syntax

Placement supports both shorthand (string) and full (object) forms:

```yaml
# Shorthand — document-level position
placement: top
placement: bottom

# Full form — anchored to section
placement:
  section: badges
  position: below

# Full form — anchored to regex match
placement:
  match: "^## Installation"
  position: above

# Full form — scoped regex within section
placement:
  section: docker-stats
  match: "^pulls: .*"
  position: replace
```

## Modules

Modules are pluggable content producers. Each module type knows how to render itself to inline markdown.

### Badge Module

Renders a markdown badge image, optionally wrapped in a link.

| Field     | Description |
|-----------|-------------|
| `alt`     | Image alt text |
| `file`    | Relative path to a committed SVG (resolved via `raw_base`) |
| `url`     | Absolute image URL (shields.io, etc.) — mutually exclusive with `file` |
| `link`    | Click target — absolute URL or relative path (resolved via `link_base`) |

```yaml
- badge: release
  file: ".stagefreight/badges/release.svg"
  link: "https://github.com/myorg/myrepo/releases"
```

### Shield Module

Shorthand for shields.io badges. The shield path is appended to `https://img.shields.io/`.

| Field     | Description |
|-----------|-------------|
| `shield`  | shields.io path (e.g., `docker/pulls/myorg/myrepo`) |
| `label`   | Override the default label (optional) |
| `link`    | Click target URL |

```yaml
- shield: docker/pulls/prplanit/stagefreight
  link: "https://hub.docker.com/r/prplanit/stagefreight"

- shield: github/actions/workflow/status/sofmeright/stagefreight/ci.yml
  label: build
  link: "https://github.com/sofmeright/stagefreight/actions"
```

Produces:
```
[![pulls](https://img.shields.io/docker/pulls/prplanit/stagefreight)](https://hub.docker.com/r/prplanit/stagefreight)
[![build](https://img.shields.io/github/actions/workflow/status/sofmeright/stagefreight/ci.yml)](https://github.com/sofmeright/stagefreight/actions)
```

### Text Module

Literal markdown text. Supports template variables.

| Field   | Description |
|---------|-------------|
| `text`  | Markdown content (supports `{version}`, `{env:VAR}`, etc.) |

```yaml
- text: "**Current version:** {version}"
- text: "Built with [StageFreight](https://github.com/sofmeright/stagefreight)"
```

### Break Module

Forces a line break between items. Items before the break are on one line, items after start a new line.

```yaml
- break:
```

### Image Module (future)

Inline image from URL or relative path.

## Composition

The narrator composes modules into rows:

- **Items** are space-joined until a `break:` forces a new line.
- **`break:`** items start a new row.

```yaml
items:
  - badge: release
    link: "https://..."
  - badge: license
    link: LICENSE
  - shield: docker/pulls/prplanit/stagefreight
    link: "https://hub.docker.com/..."
  - break:
  - shield: github/actions/workflow/status/sofmeright/stagefreight/ci.yml
    label: build
  - text: "— {version}"
```

Produces:
```
[![release](...)](...) [![license](...)](...) [![pulls](https://img.shields.io/...)](...)
[![build](https://img.shields.io/...)](...) — 1.2.3
```

## Template Resolution

All narrator modules support template variables. Templates are resolved universally across every module type — badge values, text content, shield labels, etc.

| Template | Description |
|----------|-------------|
| `{version}` | Full semantic version (e.g., `1.2.3`) |
| `{major}` | Major version number |
| `{minor}` | Minor version number |
| `{patch}` | Patch version number |
| `{major}.{minor}` | Major.minor (composable) |
| `{sha}` | Full commit SHA |
| `{sha:N}` | First N characters of SHA (e.g., `{sha:7}`) |
| `{branch}` | Current branch name |
| `{env:VAR}` | Environment variable value |

## URL Resolution

Badge `file` and `link` paths are resolved using base URLs:

| Field | Resolves via | Purpose |
|-------|-------------|---------|
| `file` | `raw_base` | Image source — needs raw file access |
| `link` (relative) | `link_base` | Click target — rendered page URL |
| `url` | (used as-is) | Absolute URL |
| `link` (absolute) | (used as-is) | Absolute URL |

### `raw_base` Auto-Derivation

If `raw_base` is not set, it is derived from `link_base`:

| Forge | `link_base` | Derived `raw_base` |
|-------|-------------|---------------------|
| GitHub | `github.com/{owner}/{repo}/blob/{branch}` | `raw.githubusercontent.com/{owner}/{repo}/{branch}` |
| GitLab | `gitlab.com/{owner}/{repo}/-/blob/{branch}` | `gitlab.com/{owner}/{repo}/-/raw/{branch}` |
| Gitea  | `{host}/{owner}/{repo}/src/branch/{branch}` | `{host}/{owner}/{repo}/raw/branch/{branch}` |

Set `raw_base` explicitly for self-hosted forges or non-standard URL patterns.

## Badge Generation

The badge asset pipeline is separate from composition. Generation creates SVG files; the narrator composes them into documents.

### Global Defaults + Per-Item Overrides

```yaml
badges:
  font: dejavu-sans              # default for all items
  font_size: 11                  # default for all items
  # font_file: ./Custom.ttf      # custom font file (overrides font)
  items:
    - name: release
      label: release
      value: "{version}"         # gitver templates: {version}, {major}.{minor}, {sha:7}
      color: auto                # auto = status-driven, or hex "#4c1"
      output: .stagefreight/badges/release.svg
      # uses global font defaults

    - name: license
      label: license
      value: "AGPL-3.0"
      color: "#007ec6"
      font: monofur              # per-badge font override
      font_size: 13              # per-badge size override
      output: .stagefreight/badges/license.svg

    - name: fancy
      label: built with
      value: "stagefreight"
      color: "#8a2be2"
      font_file: ./fonts/Custom.ttf  # per-badge custom file
      output: .stagefreight/badges/fancy.svg
```

### Built-In Fonts

`dejavu-sans` (default), `vera`, `monofur`, `vera-mono`, `ethereal`, `playpen-sans`, `doto`, `penyae`

All fonts — built-in and custom — go through the same `sfnt` + `opentype` measurement code path. Built-in fonts are just embedded TTF files. Every font gets base64 `@font-face` embedded in the SVG for pixel-perfect rendering everywhere.

## Full Config Reference

```yaml
# Badge generation (asset pipeline)
badges:
  font: dejavu-sans
  font_size: 11
  items:
    - name: release
      label: release
      value: "{version}"
      color: auto
      output: .stagefreight/badges/release.svg
    - name: license
      label: license
      value: "AGPL-3.0"
      color: "#007ec6"
      output: .stagefreight/badges/license.svg

# Narrator composition (top-level)
narrator:
  link_base: "https://github.com/myorg/myrepo/blob/main"
  # raw_base: auto-derived from link_base (or set explicitly)
  files:
    - path: README.md
      sections:
        # Badge row at the top
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

        # Docker stats section after "## Docker" heading
        - name: docker-stats
          placement:
            match: "^## Docker"
            position: below
          items:
            - shield: docker/image-size/myorg/myrepo
              link: "https://hub.docker.com/r/myorg/myrepo"
            - text: "Pulls: {env:DOCKER_PULLS}"

        # Inline version in a table cell
        - name: api-version
          inline: true
          items:
            - text: "{version}"

        # Plain text heading (no markers)
        - name: title
          plain: true
          placement: top
          items:
            - text: "# My Project"

        # Replace a specific line inside another section
        - name: pulls-count
          placement:
            section: docker-stats
            match: "^Pulls: .*"
            position: replace
          items:
            - text: "Pulls: 12,847"

# Docker readme sync (uses narrator for badge injection)
docker:
  readme:
    link_base: "https://github.com/myorg/myrepo/blob/main"
    badges:
      - alt: release
        file: ".stagefreight/badges/release.svg"
        link: "https://github.com/myorg/myrepo/releases"
      - alt: license
        file: ".stagefreight/badges/license.svg"
        link: LICENSE
      - alt: pulls
        url: "https://img.shields.io/docker/pulls/myorg/myrepo"
        link: "https://hub.docker.com/r/myorg/myrepo"
      - alt: docker-size
        url: "https://img.shields.io/docker/image-size/myorg/myrepo"
        section: docker-stats    # targets <!-- sf:docker-stats --> instead of default
```

## Shell Usage

The narrator can be driven entirely from the shell, enabling template-based README generation:

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

# Run all narrator sections from config
stagefreight narrator run

# Dry-run to preview changes
stagefreight narrator run --dry-run
```
