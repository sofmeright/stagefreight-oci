# TODO: CI Pipeline Output Overhaul + Cache Relocation

> **Temporary file** — delete after all features below are implemented.

---

## 1. Lint Cache Relocation

**Problem:** Cache currently dumps one JSON file per file-per-module into `.stagefreight/cache/lint/` in the project directory. Nobody wants that.

**Design (golangci-lint pattern):**

```
Default:    os.UserCacheDir()/stagefreight/<project-hash>/lint/
Override:   STAGEFREIGHT_CACHE_DIR env var (absolute path)
Config:     cache_dir in .stagefreight.yml (relative to project root)
Precedence: env var > config > default
```

- `os.UserCacheDir()` — stdlib, cross-platform, XDG-aware (`~/.cache/` Linux, `~/Library/Caches/` macOS)
- `<project-hash>` — SHA256 of absolute repo root path, isolates per-project
- Keep content-addressed flat files with 2-char prefix bucketing (same as git objects)
- Kill `EnsureGitignore()` — no longer touches project directory
- `STAGEFREIGHT_CACHE_DIR` lets CI users point at an artifactable path:

```yaml
# GitLab CI example
variables:
  STAGEFREIGHT_CACHE_DIR: .cache/stagefreight
cache:
  paths:
    - .cache/stagefreight/
```

**Files to modify:**
- `src/lint/cache.go` — new `DefaultCacheDir()`, env/config precedence, remove `EnsureGitignore`
- `src/config/config.go` — add `CacheDir` field
- `src/cli/cmd/docker_build.go` — pass resolved cache dir

---

## 2. ASCII Banner with Chafa-Rendered Logo

**Concept:** The `assets/logo.png` (teal elephant carrying shipping containers with movie clapperboard) gets rendered via chafa at build time. The clapperboard area carries live pipeline context.

```
    ┌─────────────────────────────────────────┐
    │  (chafa-rendered elephant + containers  │
    │   with clapperboard on top)             │
    │                                         │
    │   Clapperboard area shows:              │
    │     STAGEFREIGHT v0.2.1                 │
    │     dev-07dfdf4                         │
    └─────────────────────────────────────────┘
```

**Implementation:**
1. Create `assets/logo-banner.png` — version with blank/template clapperboard text area
2. Pre-render with chafa at build time → embed as ANSI string constant in binary
3. At runtime, splice version/tag text onto the known clapperboard line positions
4. Fallback to plain text header for no-color terminals (`NO_COLOR`, `TERM=dumb`, piped output)
5. Banner prints once at the top of `docker build` output, before the context block

**Files to create:**
- `src/output/banner.go` — banner rendering, line splicing, fallback
- Build step in Dockerfile to run chafa and embed output

---

## 3. CI Pipeline Output Format

The entire `docker build` pipeline output gets the sectioned box-drawing treatment. Each phase gets a framed section with structured content. The build section streams layers live.

### Full Output Mockup

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

### Section Behavior

- **Build section streams live** — each layer line appears as buildx processes it. `cached` = buildx cache hit, otherwise wall-clock time. `base` lines show pulled vs cached base images. `result` line shows final image + size.
- **Lint section** shows per-module breakdown: files scanned, cache hits, findings count per module. Requires the lint engine to track per-module stats (not just global CacheHits).
- **Summary** aggregates all phases with status icons and a total wall time.
- **GitLab sections** still wrap each phase for collapsibility — the box-drawing renders inside them.
- **Color** — section headers get dim/cyan, status icons green/red/yellow, `cached` gets gray. Respects `NO_COLOR` / `TERM=dumb`.

### Files to modify/create

- `src/output/section.go` (NEW) — `Section` type with `Header()`, `Row()`, `Separator()`, `Footer()`, `Close()` methods. Handles box-drawing characters, column alignment, elapsed time in header.
- `src/output/ci.go` — new `ContextBlock()` replacing `CIHeader()`. Prints Pipeline/Commit/Branch/Platforms/Registries in aligned key-value pairs.
- `src/output/output.go` — `LintTable()` method for per-module breakdown table.
- `src/cli/cmd/docker_build.go` — wire new section format into each phase. Pass per-module stats from lint engine.
- `src/cli/cmd/badge_generate.go` — emit badge rows in section format when called from build pipeline.
- `src/lint/engine.go` — track per-module file count, cache hits, finding count (new `ModuleStats` type).
- `src/build/buildx.go` — parse buildx output stream for layer-by-layer progress (build section streaming).

### Data Flow for Per-Module Lint Stats

Current `engine.Run()` returns flat `[]Finding` with global `CacheHits` counter. Need:

```go
type ModuleStats struct {
    Name     string
    Files    int
    Cached   int
    Findings int
    Critical int
    Warnings int
}

func (e *Engine) RunWithStats(ctx, files) ([]Finding, []ModuleStats, error)
```

Each goroutine increments per-module counters instead of (or in addition to) the global atomic.

---

## 4. Template System Expansion

### Scoped Versions (tag prefix namespacing)

Multiple artifacts in the same repo can have independent version numbers via git tag prefixes:

```
v0.2.1              → Docker CLI release
component-v1.0.3    → Component release
```

Template syntax: `{version:PREFIX}` looks for `PREFIX-v*` tags instead of plain `v*` tags.

```yaml
value: "{version}"              # → 0.2.1 (default, unscoped v* tags)
value: "{version:component}"    # → 1.0.3 (from component-v* tags)
```

All version sub-fields support scoping:
```
{version:SCOPE}      {base:SCOPE}       {major:SCOPE}
{minor:SCOPE}        {patch:SCOPE}      {prerelease:SCOPE}
```

**Implementation:** `gitver.DetectVersion` gets an optional `scope` parameter. When scoped, `git describe --tags --match "SCOPE-v*"` filters to the right tag namespace. `ResolveTemplate` parses `{version:SCOPE}` patterns and calls `DetectVersion` with the scope.

### Full Template Reference

#### Already implemented (static)
```
{version}              → 0.2.1 (from git tag)
{base}                 → 0.2.1 (semver base, no prerelease)
{major}                → 0
{minor}                → 2
{patch}                → 1
{prerelease}           → alpha.1 or ""
{branch}               → main
{sha}                  → abc1234 (default 7)
{sha:N}                → truncated to N chars
{env:VAR_NAME}         → environment variable
{rand:N}               → random digits
{randhex:N}            → random hex
{n:N}                  → sequential counter (tag system)
{hex:N}                → hex counter (tag system)
```

#### Scoped versions (new — static, git I/O)
```
{version:PREFIX}       → version from PREFIX-v* tags
{base:PREFIX}          → semver base from PREFIX-v* tags
{major:PREFIX}         → major from PREFIX-v* tags
{minor:PREFIX}         → etc.
{patch:PREFIX}
{prerelease:PREFIX}
```

#### Docker/Registry (new — API-resolved at runtime)
```
{docker.pulls}                    → "1.2k" (formatted)
{docker.pulls:raw}                → "1247" (raw number)
{docker.stars}                    → star count
{docker.size}                     → "72.4 MB" (compressed image size)
{docker.size:raw}                 → "75890432" (bytes)
{docker.latest}                   → latest tag digest short hash
```

Resolves against the first `docker.io` registry in config.

#### Component (new — config + GitLab API)
```
{component.version}               → shorthand for {version:component}
{component.name}                  → from first spec_files entry name
{component.catalog_url}           → GitLab catalog page URL
{component.catalog_status}        → "published" / "draft"
```

#### Project metadata (new — git/config)
```
{project.name}                    → repo name (from git remote or config)
{project.url}                     → repo URL
{project.license}                 → SPDX identifier (LICENSE file detection)
{project.description}             → from config
{project.language}                → auto-detected
```

#### Time (new — static)
```
{date}                            → 2026-02-24 (ISO date)
{date:FORMAT}                     → custom format (Go time layout)
{datetime}                        → 2026-02-24T15:04:05Z
{timestamp}                       → unix epoch
{commit.date}                     → date of HEAD commit
```

#### CI context (new — from CI env vars, portable)
```
{ci.pipeline}                     → pipeline/run ID
{ci.runner}                       → runner name
{ci.job}                          → job name
{ci.url}                          → link to pipeline/run
```

### Resolution Tiers

| Tier | Resolution | Examples | Cost |
|------|-----------|----------|------|
| **Static** | String replacement, no I/O | version, sha, branch, env, date, ci.* | Free |
| **Config** | Read from `.stagefreight.yml` or local files | project.*, component.name | Free |
| **Git** | Shell out to git | version:SCOPE, commit.date | ~10ms per scope |
| **API** | HTTP call to external service | docker.pulls, docker.size, catalog_* | Network I/O |

API tier rules:
- Lazy — only resolved if the template is actually referenced
- Cached per run — one API call per provider, not per badge
- Graceful failure — badge shows `?` or raw template if API unreachable

### Files modified
- `src/gitver/gitver.go` — DONE: `DetectScopedVersion(rootDir, scope)` using `git describe --tags --match "SCOPE-v*"`
- `src/gitver/template.go` — DONE: scoped versions, `{date}`, `{datetime}`, `{timestamp}`, `{ci.*}` resolvers (all inline)
- `src/gitver/providers.go` (NEW) — API provider interface for docker.pulls, component.catalog_*

---

## Implementation Order

1. **Cache relocation** — DONE
2. **Section rendering** (`src/output/section.go`) — DONE
3. **Context block** — DONE (replaces `CIHeader()`)
4. **Per-module lint stats** — DONE (`RunWithStats` + `LintTable`)
5. **Phase sections** — DONE (detect/plan/build/push/readme/retention/summary)
6. **Scoped versions** — DONE (`DetectScopedVersion` + template parser in `template.go`)
7. **Static templates** — DONE (date/time, ci.* in `template.go`; project.* deferred to API tier)
8. **API templates** — docker.pulls, docker.size, component.catalog_* (needs provider interface)
9. **Build streaming** — parse buildx output for layer-by-layer progress (hardest piece)
10. **Banner** — chafa rendering + clapperboard text splicing
