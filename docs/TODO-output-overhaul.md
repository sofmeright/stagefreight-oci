# TODO: CI Pipeline, Output, Templates, Narrator & Badge Pipeline

> **Temporary file** — delete after all features below are implemented.

---

## 1. Lint Cache Relocation — DONE

Moved cache from `.stagefreight/cache/lint/` to XDG-aware `os.UserCacheDir()/stagefreight/<project-hash>/lint/`.
Override with `STAGEFREIGHT_CACHE_DIR` env var or `cache_dir` in `.stagefreight.yml`.

---

## 2. Section Rendering — DONE

`src/output/section.go` — `Section` type with `NewSection()`, `Row()`, `Separator()`, `Close()`.
`ContextBlock()` replaces `CIHeader()`. `StatusIcon()`, `SummaryRow()`, `SummaryTotal()`.

---

## 3. Per-Module Lint Stats — DONE

`RunWithStats()` in `src/lint/engine.go` returns `[]ModuleStats` alongside findings.
`LintTable()` in `src/output/output.go` renders per-module breakdown.

---

## 4. Phase Sections — DONE

`docker_build.go` uses section format for detect/plan/build/push/readme/retention/summary.

---

## 5. Scoped Versions — DONE

`DetectScopedVersion(rootDir, scope)` in `src/gitver/gitver.go`.
`{version:SCOPE}`, `{base:SCOPE}`, etc. in `src/gitver/template.go`.

---

## 6. Static Templates (date/time, CI context) — DONE

`{date}`, `{datetime}`, `{timestamp}` — `resolveTime()` in `template.go`.
`{ci.pipeline}`, `{ci.runner}`, `{ci.job}`, `{ci.url}` — `resolveCIContext()` in `template.go`.
Portable env var mapping: GitLab, GitHub Actions, Jenkins, Bitbucket.

---

## 7. Badges Section in Build Pipeline

**Status: TODO**

The mockup shows a `── Badges ──` section as part of `docker build` pipeline output.
Currently badge generation runs as a separate CI stage — it needs to also be wirable
as an inline phase in the build pipeline.

```
    ── Badges ──────────────────────────────────────────── 4ms ──
    │ release         .badges/release.svg    monofur  11pt  #74ecbe
    │ build           .badges/build.svg      monofur  11pt  auto
    │ license         .badges/license.svg    monofur  11pt  #310937
    │ updated         .badges/updated.svg    dejavu   11pt  #236144
    └─────────────────────────────────────────────────────────────
```

**Implementation:**
- Add `runBadgeSection()` to `docker_build.go` — calls badge engine, emits section rows
- `badge_generate.go` output should use section format (currently plain `fmt.Printf`)
- Summary line: `badges ✓ 4 generated`

**Files to modify:**
- `src/cli/cmd/docker_build.go` — add badge phase between push and readme
- `src/cli/cmd/badge_generate.go` — section-formatted output

---

## 8. Remaining Template Variables

**Status: TODO**

### `{date:FORMAT}` — custom date formatting
Currently `{date}` is hardcoded to `2006-01-02`. Need `{date:FORMAT}` where FORMAT
is a Go time layout string (e.g. `{date:Jan 2 2006}`, `{date:20060102}`).

**File:** `src/gitver/template.go` — extend `resolveTime()` to parse `{date:...}`.

### `{commit.date}` — HEAD commit date
Date of the HEAD commit (not current time). Needs `git log -1 --format=%aI HEAD`.

**File:** `src/gitver/template.go` — new resolver, needs rootDir for git call.

### Project metadata (Config/Git tier)
```
{project.name}            → repo name (from git remote or config)
{project.url}             → repo URL (from git remote or config)
{project.license}         → SPDX identifier (LICENSE file detection)
{project.description}     → from .stagefreight.yml description field
{project.language}        → auto-detected (reuse build detect logic)
```

These are Config or Git tier (no network I/O), not API tier.

**Files:**
- `src/gitver/template.go` — `resolveProjectMeta()` resolver
- `src/gitver/project.go` (NEW) — git remote parsing, LICENSE detection, language detection
- `src/config/config.go` — may need `Project` section for description override

### Docker/Registry (API tier)
```
{docker.pulls}            → "1.2k" (formatted)
{docker.pulls:raw}        → "1247" (raw number)
{docker.stars}            → star count
{docker.size}             → "72.4 MB" (compressed image size)
{docker.size:raw}         → "75890432" (bytes)
{docker.latest}           → latest tag digest short hash
```

Resolves against the first `docker.io` registry in config.

### Component (Config + GitLab API tier)
```
{component.version}       → shorthand for {version:component}
{component.name}          → from first spec_files entry name
{component.catalog_url}   → GitLab catalog page URL
{component.catalog_status} → "published" / "draft"
```

### API tier rules
- Lazy — only resolved if the template is actually referenced
- Cached per run — one API call per provider, not per badge
- Graceful failure — badge shows `?` or raw template if API unreachable

**Files:**
- `src/gitver/providers.go` (NEW) — API provider interface
- `src/gitver/docker_provider.go` (NEW) — Docker Hub API client
- `src/gitver/gitlab_provider.go` (NEW) — GitLab catalog API client

---

## 9. Narrator CLI Command

**Status: TODO**

The narrator package (`src/narrator/`) exists with modules (Badge, Shield, Text, Break),
composition logic (`Compose`, `ComposeRows`), and full config support (`NarratorConfig`,
`NarratorSection`, `NarratorItem`, placement system). But there's no standalone CLI command.

### `stagefreight narrator compose`

Reads `narrator:` config, resolves templates in values, composes modules per section,
and injects/updates `<!-- sf:<name> -->` managed sections in target files.

```yaml
narrator:
  link_base: "https://github.com/sofmeright/stagefreight/blob/main"
  files:
    - path: README.md
      sections:
        - name: badges
          placement: top
          items:
            - badge: release
              file: ".badges/release.svg"
              link: "https://github.com/sofmeright/stagefreight/releases"
            - shield: "docker/pulls/prplanit/stagefreight"
              link: "https://hub.docker.com/r/prplanit/stagefreight"
```

**Implementation:**
- `src/cli/cmd/narrator.go` (NEW) — parent command
- `src/cli/cmd/narrator_compose.go` (NEW) — compose subcommand
  - Load config, detect version for template resolution
  - For each file: read content, compose each section, place via `PlaceContent()`
  - Write back (or `--dry-run` to stdout)
- Narrator compose should also be callable from `docker_build.go` pipeline as a phase

**Files:**
- `src/cli/cmd/narrator.go` (NEW)
- `src/cli/cmd/narrator_compose.go` (NEW)

---

## 10. Build Streaming

**Status: TODO** (hardest piece)

Parse buildx output stream for layer-by-layer progress display instead of
suppressing buildx output and showing only the result.

### Target output
```
    ── Build ───────────────────────────────────────── 1m32.6s ──
    │ base    golang:1.25-alpine                     pulled   3.2s
    │ base    alpine:3.21                            cached
    │ COPY    go.mod go.sum ./                       cached
    │ RUN     go mod download                        cached
    │ COPY    . .                                    0.4s
    │ RUN     go build -ldflags ... -o /stagefreight 44.8s
    │ COPY    --from=build /stagefreight /usr/bin/   cached
    │ COPY    --from=build /etc/ssl/certs/ ...       cached
    ├─────────────────────────────────────────────────────────────
    │ result  stagefreight:dev-07dfdf4               72.4 MB
    └─────────────────────────────────────────────────────────────
```

### Parsing strategy

Buildx `--progress=plain` output uses `#N [stage M/N] INSTRUCTION` format:
```
#9  [builder 1/7] FROM docker.io/library/golang:1.25-alpine@sha256:...
#13 [builder 2/7] RUN apk add --no-cache git
#16 [builder 4/7] COPY go.mod go.sum* ./
#20 [builder 7/7] RUN CGO_ENABLED=0 go build ...
#20 DONE 44.8s
```

Key patterns to match:
- `#N [stage M/N] FROM image@sha256:...` → base image pull (extract image name, track pull vs cached)
- `#N [stage M/N] COPY ...` → COPY layer
- `#N [stage M/N] RUN ...` → RUN layer (truncate long commands)
- `#N DONE Ns` → layer completion time
- `#N CACHED` → cache hit
- `exporting to image` / `writing image sha256:...` → result

**Implementation:**
- Intercept buildx stdout/stderr as a streaming `io.Reader`
- Parse lines in real-time, emit section rows as layers complete
- Track per-layer state: instruction, start time, cached flag
- Final `result` line needs image name + size (from `docker inspect` after load)

**Files to modify/create:**
- `src/build/buildx.go` — streaming output parser, `LayerProgress` type
- `src/build/parser.go` (NEW) — buildx output line parser with regex patterns
- `src/cli/cmd/docker_build.go` — wire streaming parser into build section

### Color in build streaming
- Section headers: dim cyan (already done)
- `cached`: gray (`\033[90m`)
- Layer timing: default
- `result`: bold

---

## 11. Banner

**Status: TODO**

ASCII banner with chafa-rendered logo. The `assets/logo.png` (teal elephant carrying
shipping containers with movie clapperboard) gets rendered at build time. Clapperboard
area carries live pipeline context.

**Implementation:**
1. Create `assets/logo-banner.png` — version with blank/template clapperboard text area
2. Pre-render with chafa at build time → embed as ANSI string constant in binary
3. At runtime, splice version/tag text onto known clapperboard line positions
4. Fallback to plain text header for no-color terminals (`NO_COLOR`, `TERM=dumb`, piped output)
5. Banner prints once at top of `docker build` output, before context block

**Files:**
- `src/output/banner.go` (NEW) — banner rendering, line splicing, fallback
- Dockerfile — build step to run chafa and embed output

---

## Full Output Mockup

```
    Pipeline    6009          Runner     Ant-Parade
    Commit      07dfdf4       Branch     main
    Platforms   linux/amd64   Registries 1 (docker.io)

    ── Lint ──────────────────────────────────────────── 120ms ──
    │ module          files   cached  findings
    │ dockerfile       1       0       0
    │ go-vet          14       8       0
    │ large-files     34      34       0
    │ secrets          0       0       0
    ├─────────────────────────────────────────────────────────────
    │ total           49      42       0 findings (0 critical)
    └─────────────────────────────────────────────────────────────

    ── Detect ─────────────────────────────────────────── 0ms ──
    │ Dockerfile      → go (auto-detected)
    │ context         → .
    │ target          → (default)
    └─────────────────────────────────────────────────────────────

    ── Plan ──────────────────────────────────────────── 16ms ──
    │ platforms       linux/amd64
    │ tags            dev-07dfdf4, latest-dev
    │ strategy        load + push
    └─────────────────────────────────────────────────────────────

    ── Build ───────────────────────────────────────── 1m32.6s ──
    │ base    golang:1.25-alpine                     pulled   3.2s
    │ base    alpine:3.21                            cached
    │ COPY    go.mod go.sum ./                       cached
    │ RUN     go mod download                        cached
    │ COPY    . .                                    0.4s
    │ RUN     go build -ldflags ... -o /stagefreight 44.8s
    │ COPY    --from=build /stagefreight /usr/bin/   cached
    │ COPY    --from=build /etc/ssl/certs/ ...       cached
    ├─────────────────────────────────────────────────────────────
    │ result  stagefreight:dev-07dfdf4               72.4 MB
    └─────────────────────────────────────────────────────────────

    ── Push ──────────────────────────────────────────── 38.6s ──
    │ docker.io/prplanit/stagefreight:dev-07dfdf4          ✓
    │ docker.io/prplanit/stagefreight:latest-dev           ✓
    └─────────────────────────────────────────────────────────────

    ── Badges ──────────────────────────────────────────── 4ms ──
    │ release         .badges/release.svg    monofur  11pt  #74ecbe
    │ build           .badges/build.svg      monofur  11pt  auto
    │ license         .badges/license.svg    monofur  11pt  #310937
    │ updated         .badges/updated.svg    dejavu   11pt  #236144
    └─────────────────────────────────────────────────────────────

    ── Readme ──────────────────────────────────────── 210ms ──
    │ docker.io/prplanit/stagefreight       synced (4 badges)
    └─────────────────────────────────────────────────────────────

    ── Retention ───────────────────────────────────── 800ms ──
    │ docker.io/prplanit/stagefreight       kept 6, pruned 2
    │   - dev-a1b2c3d
    │   - dev-e4f5g6h
    └─────────────────────────────────────────────────────────────

    ── Summary ───────────────────────────────────────────────────
    │ lint        ✓  49 files, 42 cached, 0 critical
    │ detect      ✓  1 Dockerfile(s), go
    │ plan        ✓  linux/amd64, 2 tag(s), load+push
    │ build       ✓  1 image(s), 72.4 MB
    │ push        ✓  2 tag(s) → 1 registry
    │ badges      ✓  4 generated
    │ readme      ✓  1 synced
    │ retention   ✓  kept 6, pruned 2
    ├─────────────────────────────────────────────────────────────
    │ total                                          2m12.4s   ✓
    └─────────────────────────────────────────────────────────────

    Image References
    → docker.io/prplanit/stagefreight:dev-07dfdf4
    → docker.io/prplanit/stagefreight:latest-dev
```

---

## Template Reference

### Implemented

| Template | Tier | Description |
|----------|------|-------------|
| `{version}` | Static | Full version from git tag |
| `{base}` | Static | Semver base (no prerelease) |
| `{major}`, `{minor}`, `{patch}` | Static | Semver components |
| `{prerelease}` | Static | Prerelease suffix or empty |
| `{branch}` | Static | Current branch |
| `{sha}`, `{sha:N}` | Static | Commit SHA (default 7, or N chars) |
| `{env:VAR}` | Static | Environment variable |
| `{rand:N}`, `{randhex:N}` | Static | Random digits/hex |
| `{n:N}`, `{hex:N}` | Static | Sequential counters (tag system) |
| `{version:SCOPE}` | Git | Version from SCOPE-v* tags |
| `{base:SCOPE}`, `{major:SCOPE}`, etc. | Git | Scoped semver fields |
| `{date}` | Static | ISO date (UTC) |
| `{datetime}` | Static | RFC3339 timestamp |
| `{timestamp}` | Static | Unix epoch |
| `{ci.pipeline}` | Static | Pipeline/run ID (portable) |
| `{ci.runner}` | Static | Runner/agent name (portable) |
| `{ci.job}` | Static | Job name (portable) |
| `{ci.url}` | Static | Pipeline URL (portable) |

### TODO

| Template | Tier | Description |
|----------|------|-------------|
| `{date:FORMAT}` | Static | Custom Go time layout |
| `{commit.date}` | Git | HEAD commit date |
| `{project.name}` | Config/Git | Repo name |
| `{project.url}` | Config/Git | Repo URL |
| `{project.license}` | Config | SPDX identifier |
| `{project.description}` | Config | From config |
| `{project.language}` | Config | Auto-detected |
| `{docker.pulls}` | API | Pull count (formatted) |
| `{docker.pulls:raw}` | API | Pull count (raw) |
| `{docker.stars}` | API | Star count |
| `{docker.size}` | API | Image size (formatted) |
| `{docker.size:raw}` | API | Image size (bytes) |
| `{docker.latest}` | API | Latest tag digest |
| `{component.version}` | Git | Shorthand for `{version:component}` |
| `{component.name}` | Config | Component name |
| `{component.catalog_url}` | API | GitLab catalog URL |
| `{component.catalog_status}` | API | Published/draft |

---

## Implementation Order

1. **Cache relocation** — DONE
2. **Section rendering** — DONE (`src/output/section.go`)
3. **Context block** — DONE (replaces `CIHeader()`)
4. **Per-module lint stats** — DONE (`RunWithStats` + `LintTable`)
5. **Phase sections** — DONE (detect/plan/build/push/readme/retention/summary)
6. **Scoped versions** — DONE (`DetectScopedVersion` + template parser)
7. **Static templates** — DONE (date/time, ci.* in `template.go`)
8. **Badges in pipeline** — add `── Badges ──` section to `docker build` output
9. **`{date:FORMAT}` + `{commit.date}`** — extend `resolveTime()`, add git date resolver
10. **Project metadata templates** — `{project.*}` (Config/Git tier, no network)
11. **Narrator CLI** — `stagefreight narrator compose` command
12. **API templates** — `{docker.*}`, `{component.*}` (needs provider interface + HTTP clients)
13. **Build streaming** — parse buildx output for layer-by-layer progress (hardest piece)
14. **Banner** — chafa rendering + clapperboard text splicing
