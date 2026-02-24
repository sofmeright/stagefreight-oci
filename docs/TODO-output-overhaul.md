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

## 7. Badges Section in Build Pipeline — DONE

`runBadgeSection()` in `docker_build.go` — generates all configured badges inline,
emits section rows with name/output/font/size/color. Runs between push and readme.
Summary row: `badges ✓ N generated`.

---

## 8. Remaining Template Variables

### `{date:FORMAT}` — custom date formatting — DONE
`resolveDateFormat()` in `template.go`. Go time layout string (e.g. `{date:20060102}`).
Also fixed pre-existing bug: `{date}` replacement was clobbering `{datetime}` (substring match).

### `{commit.date}` — HEAD commit date — DONE
`resolveCommitDate()` in `template.go`. Uses `git log -1 --format=%aI HEAD`.
Requires rootDir, called from `ResolveTemplateWithDir` when rootDir is available.

### Project metadata (Config/Git tier) — DONE
`src/gitver/project.go` — `DetectProject(rootDir)`, `repoNameFromRemote()`, `remoteToHTTPS()`,
`detectLicense()` (SPDX matching), `detectLanguage()`.
`resolveProjectMeta()` in `template.go` — auto-detects name/url/license/language from git+fs.
`{project.description}` sourced via `SetProjectDescription()` (called from `docker_build.go`
with `docker.readme.description` config value).

### Docker/Registry (API tier) — DONE
`src/gitver/docker_hub.go` — `FetchDockerHubInfo(ns, repo)`, `ResolveDockerTemplates()`,
`formatCount()`, `formatBytes()`, `shortDigest()`.
Lazy: only fetches from Docker Hub API if any badge value contains `{docker.`.
Cached per run: one fetch, reused across all badges.
Wired into both `runBadgeSection()` (build pipeline) and `generateConfigBadges()` (CLI).
Uses first `docker.io` registry from config for namespace/repo.

### Component (Config + GitLab API tier)
```
{component.version}       → shorthand for {version:component}
{component.name}          → from first spec_files entry name
{component.catalog_url}   → GitLab catalog page URL
{component.catalog_status} → "published" / "draft"
```

**Status: TODO** — needs GitLab API client

**Files:**
- `src/gitver/gitlab_provider.go` (NEW) — GitLab catalog API client

---

## 9. Narrator CLI — DONE

Two commands:
- `narrator compose` — ad-hoc shell mode, type:value item pairs with placement flags
- `narrator run` — config-driven, reads `narrator.files` config and processes all sections

Both resolve templates, build narrator modules (badge/shield/text/break), compose via
`narrator.Compose()`, and place via `PlaceContent()` with idempotent section replacement.

Files: `narrator.go`, `narrator_compose.go`, `narrator_run.go`

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
| `{date:FORMAT}` | Static | Custom Go time layout |
| `{commit.date}` | Git | HEAD commit date |
| `{project.name}` | Git | Repo name from git remote |
| `{project.url}` | Git | Repo URL (SSH→HTTPS conversion) |
| `{project.license}` | Filesystem | SPDX identifier from LICENSE |
| `{project.description}` | Config | From SetProjectDescription |
| `{project.language}` | Filesystem | Auto-detected from lockfiles |
| `{docker.pulls}` | API | Pull count (formatted, e.g. "1.2k") |
| `{docker.pulls:raw}` | API | Pull count (raw number) |
| `{docker.stars}` | API | Star count |
| `{docker.size}` | API | Image size (formatted, e.g. "72.4 MB") |
| `{docker.size:raw}` | API | Image size (bytes) |
| `{docker.latest}` | API | Latest tag digest (12 chars) |

### TODO

| Template | Tier | Description |
|----------|------|-------------|
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
8. **Badges in pipeline** — DONE (`runBadgeSection` in `docker_build.go`)
9. **`{date:FORMAT}` + `{commit.date}`** — DONE (`resolveDateFormat`, `resolveCommitDate`, `{datetime}` bug fix)
10. **Project metadata templates** — DONE (`{project.*}` in `project.go` + `resolveProjectMeta`)
11. **Narrator CLI** — DONE (`narrator compose` ad-hoc + `narrator run` config-driven)
12. **Docker Hub API templates** — DONE (`{docker.*}` in `docker_hub.go` + lazy fetch)
13. **Component templates** — `{component.*}` (needs GitLab API client)
14. **Build streaming** — parse buildx output for layer-by-layer progress (hardest piece)
15. **Banner** — chafa rendering + clapperboard text splicing
