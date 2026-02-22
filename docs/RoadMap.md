# StageFreight — Road Map

> **Repository Lifecycle Steward**
> A daemon that watches over your entire repo portfolio and actively maintains it.

StageFreight is evolving from a GitLab CI component with bash scripts into a platform-agnostic **Repository Lifecycle Steward**. This document captures the full vision — from the Go CLI rewrite through daemon mode — absorbing the [prplangit concept](https://gitlab.prplanit.com/precisionplanit/dungeon/-/blob/main/docs/Ideas.md) under the StageFreight brand.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  docker.io/prplanit/stagefreight                            │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  /usr/local/bin/stagefreight   (Go binary)            │  │
│  │                                                       │  │
│  │  subcommands:                                         │  │
│  │    init                                               │  │
│  │    release notes | release create                     │  │
│  │    docs inputs                                        │  │
│  │    badge update                                       │  │
│  │    docker build | build                               │  │
│  │    lint                                               │  │
│  │    security scan                                      │  │
│  │    dev build | dev test | dev up                      │  │
│  │    sync readme | sync metadata                        │  │
│  │    lint links                                         │  │
│  │    flair                                              │  │
│  │    fork sync | fork status | fork audit               │  │
│  │    audit deps | audit packages | audit codebase        │  │
│  │    sbom diff                                          │  │
│  │    serve           ← daemon mode                      │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
│  Alpine base + docker-cli + buildx (existing toolchain)     │
└─────────────────────────────────────────────────────────────┘

Consumers:
  ┌──────────────────────┐    ┌──────────────────────────┐
  │  GitLab CI component │    │  GitHub Actions wrapper   │
  │  (thin YAML calling  │    │  (thin YAML calling       │
  │   the binary)        │    │   the binary)             │
  └──────────────────────┘    └──────────────────────────┘
  ┌──────────────────────────────────────────────────────┐
  │  stagefreight serve  (K8s pod / daemon mode)         │
  │  — org scanning, manifest discovery, scheduled jobs  │
  └──────────────────────────────────────────────────────┘
```

**Key architectural decisions:**

- **Go binary** — all logic lives in `stagefreight`, the single Go CLI. Consistency with internal tooling (hasteward), mature DevOps ecosystem (go-git, go-gitlab, go-github, go-sdk), single-binary distribution.
- **Provider abstraction** — GitLab, GitHub, Gitea/Forgejo behind a common interface. CLI subcommands work against any provider; daemon mode connects to multiple simultaneously.
- **OCI image** — same image (`docker.io/prplanit/stagefreight`), now ships the Go binary alongside the existing Alpine toolchain. CI components become thin YAML wrappers that call the binary.
- **MR-first** — every write operation produces a merge/pull request, never direct commits. Unless YOLO mode is on.

---

## Adoption — Existing Projects with Minimal Friction

StageFreight is useless if onboarding an existing project means rewriting CI pipelines, restructuring Dockerfiles, or committing a 50-line manifest before anything works. The adoption model is designed around **zero friction at entry, progressive depth as you want it.**

### Zero-Config CLI (No Manifest Required)

Every CLI subcommand works without a `.stagefreight.yml`. The binary inspects the repo and infers sensible defaults:

```
$ cd my-existing-project
$ stagefreight release notes              # scans git log, generates release notes
$ stagefreight docker build --local       # finds Dockerfile, builds for local platform
$ stagefreight security scan              # finds Dockerfile or lockfiles, runs Trivy
$ stagefreight dev test                   # no dev.test config? runs Dockerfile healthcheck if defined
```

**How inference works:**
- **Dockerfile** — found by walking the repo root, `build/`, `docker/`, `.` for `Dockerfile`, `Dockerfile.*`, `*.dockerfile`. If multiple exist, pick the root one or error with a clear message.
- **Language/toolchain** — detected from lockfiles (`go.mod`, `package.json`, `Cargo.toml`, `requirements.txt`, `*.csproj`, etc.). Informs security scanning, `.gitignore` lint, license header patterns.
- **Git provider** — detected from `git remote` origin URL. GitLab, GitHub, Gitea recognized automatically.
- **Existing CI** — StageFreight does not touch or conflict with existing `.gitlab-ci.yml` / `.github/workflows/`. It runs alongside them. You can adopt one subcommand at a time without disrupting anything.
- **Existing Dockerfiles** — used as-is. StageFreight never modifies, generates, or rewrites Dockerfiles. Your multistage builds, your base images, your build args — all untouched. StageFreight is the orchestrator, not the author.
- **Healthcheck fallback** — if no `dev.test` config exists but the Dockerfile has a `HEALTHCHECK` instruction, `stagefreight dev test` uses it. If the image exposes ports, it probes them. Something useful happens without any config.

### `stagefreight init` — Scaffold a Manifest from What Exists

For projects that want a manifest but don't want to write one from scratch:

```
$ stagefreight init
Detected:
  Language:    Go (go.mod)
  Dockerfile:  ./Dockerfile (multistage: builder, production, dev)
  Provider:    GitLab (gitlab.prplanit.com)
  CI:          .gitlab-ci.yml (existing — will not modify)
  Registry:    registry.prplanit.com/tools/my-app (from CI variables)

Generated .stagefreight.yml:

  docker:
    dockerfile: Dockerfile
    target: production
    platforms: [linux/amd64]
    registries:
      - url: registry.prplanit.com
        path: tools/my-app
        tags: ["{version}", latest]
  security:
    scan: true
  dev:
    target: dev              # detected "dev" stage in Dockerfile
    test:
      healthchecks:
        - { name: http, type: http, url: "http://localhost:8080/", expect_status: 200 }

Creating branch stagefreight/init...
Opening MR against main...
  → https://gitlab.prplanit.com/precisionplanit/my-app/-/merge_requests/42

$ stagefreight init --local   # alternative: just write the file, no git operations
Write .stagefreight.yml? [Y/n]
```

`init` reads everything that already exists — Dockerfile stages, CI config, git remotes, exposed ports, HEALTHCHECK instructions, existing registry references — and produces a manifest that describes what the project already does. No behavior change on day one, just a declarative description of the status quo that you can then evolve.

**Default behavior: creates an MR.** The manifest is committed to a `stagefreight/init` branch and an MR is opened against the default branch. This is the MR-first philosophy — the repo owner reviews what StageFreight detected, adjusts if needed, and merges when ready. The MR description explains what was inferred and why.

**Flags:**
- `stagefreight init` — creates MR with the generated manifest (default, MR-first)
- `stagefreight init --commit` — skip the MR, commit `.stagefreight.yml` directly to the current branch (for solo devs, YOLO users, or repos where you're the only maintainer)
- `stagefreight init --branch <name>` — target a specific branch. With `--commit`, commits directly to that branch. Without `--commit`, opens the MR against that branch as the base. Useful for dropping the manifest into a feature branch first to test before it hits main.
- `stagefreight init --local` — write the file to disk only, no git operations (for inspection before committing yourself)
- `stagefreight init --dry-run` — print what it would generate without writing anything
- `stagefreight init --full` — include all config sections with commented-out defaults as documentation
- `stagefreight init --interactive` — walk through each section interactively, asking what to enable

**Daemon mode:** when `stagefreight serve` discovers a repo with no `.stagefreight.yml` but org defaults are configured, it can auto-open an `init` MR to adopt the repo. The MR contains the generated manifest based on what the daemon inferred + org defaults applied. Repo owner merges to opt in, closes to opt out. This is how org-wide rollout works without touching every repo manually.

### Progressive Adoption Path

The adoption curve is intentionally shallow:

```
Level 0 — CLI only, no manifest
  └─ "I just want release notes / a local build / a security scan"
  └─ Install the binary, run subcommands ad-hoc
  └─ Works today, nothing to configure

Level 1 — Manifest for builds
  └─ "I want reproducible builds with proper tagging"
  └─ Run `stagefreight init`, tweak the docker section
  └─ Existing CI calls `stagefreight docker build` instead of raw docker commands

Level 2 — Manifest for dev experience
  └─ "I want contributors to be able to run and test locally"
  └─ Add dev.services, dev.test.healthchecks, dev.test.sanity
  └─ `stagefreight dev up` replaces the "how to run locally" README section

Level 3 — Manifest for full lifecycle
  └─ "I want security scanning, license checks, badges, the works"
  └─ Enable sections incrementally — each one is independent
  └─ CI pipeline shrinks as StageFreight absorbs more jobs

Level 4 — Daemon mode
  └─ "I want all my repos managed automatically"
  └─ Deploy `stagefreight serve`, configure org defaults
  └─ Repos with manifests get full treatment, repos without get org defaults
  └─ Repos with no manifest and no org defaults are left alone
```

### Coexistence with Existing CI

StageFreight does not replace your CI system. It's a tool your CI calls — or that runs alongside it.

**GitLab — gradual migration:**
```yaml
# Before: raw script in .gitlab-ci.yml
build:
  image: golang:1.24-bullseye
  script: |
    apt-get update && apt-get install -y ...
    go build -o my-app
    docker build -t registry.prplanit.com/tools/my-app:$CI_COMMIT_TAG .
    docker push ...

# After: one line calling stagefreight
build:
  image: docker.io/prplanit/stagefreight:latest
  script:
    - stagefreight docker build

# Or even thinner — use the StageFreight GitLab component
include:
  - component: gitlab.prplanit.com/components/stagefreight/docker-build@main
```

Each CI job can be migrated independently. Replace the `release_notes` job first. Then `docker build`. Then `security scan`. The rest of the pipeline stays exactly as it is. No big bang rewrite.

**GitHub Actions — same pattern:**
```yaml
# Before: 40 lines of shell in a workflow
- name: Build
  run: |
    docker buildx build --platform linux/amd64,linux/arm64 ...

# After: one step
- name: Build
  uses: sofmeright/stagefreight-action@v1
  # reads .stagefreight.yml, does the right thing
```

**No CI at all — also fine:**
Projects without CI pipelines benefit immediately from the CLI. `stagefreight dev build && stagefreight dev test` gives them a reproducible build and test workflow without ever setting up GitLab CI or GitHub Actions. When they're ready for CI, the manifest they already have plugs straight in.

### Existing Dockerfiles — No Changes Required

StageFreight works with whatever Dockerfile you have today:

| What you have | What StageFreight does |
|---|---|
| Single-stage `FROM alpine ... RUN ...` | Builds it. Tags it. Pushes it. |
| Multistage `FROM node AS builder` / `FROM nginx AS production` | Builds targeting the final (or configured) stage. |
| `HEALTHCHECK` instruction | Uses it for `dev test` healthchecks if no manifest test config. |
| `EXPOSE` ports | Probes them during `dev test` if no manifest healthchecks. |
| `ARG` build args | Pass them via manifest `build_args` or CLI `--build-arg`. |
| No Dockerfile (library, docs, pure config repo) | `stagefreight build` still works for artifact builds. Everything else (release notes, security scan, badges, flair) works without any Dockerfile at all. |
| Raw CI scripts with `apt-get install` and inline builds | Works as-is today. When you're ready, wrap the toolchain in a Dockerfile and let `stagefreight build` handle it. No rush. |

---

## Phased Feature Road Map

### Phase 0 — Foundation (CLI Rewrite)

Rewrite existing bash/Python logic as Go subcommands. No new features, just parity with what exists today plus proper structure.

| Subcommand | Replaces | Description |
|---|---|---|
| `stagefreight init` | Manual manifest creation | Infer project config from existing repo, scaffold `.stagefreight.yml`, open MR to adopt |
| `stagefreight release notes` | `generate-release_notes.sh` | Git log mining, commit categorization, markdown output |
| `stagefreight release create` | GitLab API bash calls | API release creation with asset linking |
| `stagefreight docs inputs` | Embedded Python + jq | Component YAML spec → markdown table generation |
| `stagefreight badge update` | Badge bash scripts | SVG templating, pipeline status query, commit back |
| `stagefreight docker build` | Docker build bash scripts | Flexible multi-registry, multi-arch, multi-OS container image build+push engine (see below) |
| `stagefreight build` | Raw CI scripts (apt-get, wget, etc.) | Artifact builds — multi-toolchain Dockerfile pipelines that output binaries, zips, packages instead of images (see below) |
| `stagefreight lint` | Pre-commit hooks, manual checks | Cache-aware, delta-only code quality gate that runs before every build (see below) |
| `stagefreight security scan` | Trivy bash wrapper | Trivy orchestration, SBOM generation, vendor-specific result upload |

#### `stagefreight lint` — Built-In Code Quality Gate

Lint is not a feature you opt into — it's the floor. Every `stagefreight docker build`, `stagefreight build`, and `stagefreight dev build` runs lint first. Every commit through `stagefreight` passes through lint. It replaces pre-commit hooks, `.editorconfig` enforcement, and scattered CI lint jobs with a single, fast, cache-aware pass that developers never have to think about.

**Core philosophy: developer pace is sacred.** Lint exists to catch real problems, not to slow people down. Everything is cached. Only changed files are scanned. If your code didn't change and the rules didn't change, lint is a no-op instant pass. Developers should feel encouraged to build often, commit confidently, and never hesitate because "CI might fail on some formatting thing."

**What it checks:**

| Module | Detects | Default |
|---|---|---|
| `secrets` | Leaked credentials, API keys, tokens, private keys (TruffleHog + Gitleaks engine) | **on** |
| `unicode` | Invisible characters, bidi overrides, zero-width chars, homoglyphs, AI prompt injection, confusable whitespace (full `audit codebase` detection table) | **on** |
| `yaml` | YAML syntax errors, indentation issues, duplicate keys | **on** |
| `large-files` | Files exceeding configured size threshold (default 500KB) | **on** |
| `conflicts` | Unresolved merge conflict markers, case-sensitive filename collisions | **on** |
| `line-endings` | Mixed line endings, CRLF in non-Windows contexts, trailing whitespace, missing final newline | **on** |
| `tabs` | Tab characters in files that should use spaces (configurable per-language) | **on** |
| `kustomize` | Kustomize build validation — catches broken overlays before they hit the cluster | **off** (auto-enabled if `kustomization.yaml` detected) |
| `dockerfile` | Dockerfile lint — hadolint-style checks for best practices | **off** (auto-enabled if `Dockerfile` detected) |
| `shellcheck` | Shell script analysis — catches common bash/sh mistakes | **off** (auto-enabled if `.sh` files detected) |

Modules tagged **on** run by default with zero config. Modules tagged **off** auto-enable when relevant files are detected, but never force themselves on projects that don't have those file types.

**Tunable like logging verbosity:**

```
stagefreight lint                    # default: all default modules, changed files only
stagefreight lint --level full       # every module, every file, full scan
stagefreight lint --level changed    # default — only files changed since last clean lint
stagefreight lint --level off        # skip lint entirely (you asked for it)
stagefreight lint --module secrets   # run only the secrets module
stagefreight lint --no-module tabs   # run everything except tabs
```

Manifest config:
```yaml
lint:
  level: changed                    # full | changed (default) | off
  modules:
    secrets: true                   # individually toggle any module
    unicode: true
    yaml: true
    large_files: true
    large_files_max: 500KB          # module-specific tuning
    conflicts: true
    line_endings: true
    tabs: false                     # example: disable for a project that uses tabs
    kustomize: true                 # force-enable even without auto-detection
    dockerfile: true
    shellcheck: true
  exclude:                          # paths to skip (glob patterns, respects .gitignore)
    - "vendor/**"
    - "generated/**"
    - "*.min.js"
```

If `lint:` is absent from the manifest, the default is `level: changed` with all default-on modules active. You get full protection by writing nothing. You tune or disable by being explicit.

**Delta-aware scanning:**
- `level: changed` (the default) computes the file delta from the last clean lint pass
- "Changed" means: file content changed, or the lint rules that apply to that file changed
- In CI: delta is computed against the target branch (MR diff)
- Locally: delta is computed against the last successful `stagefreight lint` run (cached state)
- `stagefreight lint --all` overrides delta and scans everything (useful for initial adoption or after rule changes)

**Cache-aware builds:**

Lint results are cached per-file as a content-addressed hash of `(file content, rule config, lint engine version)`. This means:
- **File unchanged + rules unchanged** → cached pass, zero work
- **File changed + rules unchanged** → re-lint only this file
- **Rules changed** → re-lint all files affected by that rule module (not everything — just the module that changed)
- **Lint engine version bumped** → full re-lint (rare, only on StageFreight upgrades)
- **Explicit cache bust** → `stagefreight lint --no-cache` for when you need a clean sweep

The cache lives in `.stagefreight/cache/lint/` (gitignored). CI runners benefit from persistent caches across pipeline runs. The daemon maintains its own cache per repo.

**Build integration:**

Every build command (`docker build`, `build`, `dev build`) runs lint as its first step. If lint fails, the build does not start. This is not a separate CI stage — it's baked into the build itself. The cache makes this effectively free on repeated builds where nothing relevant changed.

```
$ stagefreight docker build
  lint ✓ (3 files changed, 247 cached, 0.2s)
  build ✓ linux/amd64 (cached layers, 4.1s)
  push ✓ docker.io/prplanit/myapp:1.2.3
```

When lint is clean and nothing changed, you see:
```
$ stagefreight docker build
  lint ✓ (0 files changed, 250 cached, 0.0s)
  build ✓ linux/amd64 (fully cached, 0.8s)
```

When lint fails, you see exactly what and where:
```
$ stagefreight docker build
  lint ✗ 2 issues in 1 file
    src/auth.py:14  [secrets] Possible API key: AKIAIOSFODNN7EXAMPLE
    src/auth.py:31  [unicode] Zero-width space U+200B in string literal
  build skipped — lint failed
```

**Relationship to `audit codebase`:**

`stagefreight lint --module unicode` runs the same detection engine described in Phase 4's `audit codebase` section — same character tables, same severity levels, same fix mode. The difference is scope: `lint` runs on changed files during every build; `audit codebase` is a deep scan of the entire repository on demand. Think of lint as the per-commit guard and `audit codebase` as the periodic full sweep.

#### `stagefreight docker build` — Build Engine

The build subcommand is the workhorse. Registries have wildly different conventions for image naming, tagging, and manifest structure. StageFreight abstracts all of it behind a declarative config.

**Everything is Dockerfile-native.** StageFreight does not invent its own build format — it orchestrates `docker buildx` against standard Dockerfiles. The manifest declares *what* to build and *where* to push; the Dockerfile defines *how* the image is constructed. Multistage builds, scratch images, distroless bases, vendor S6-overlay patterns — whatever your Dockerfile does, StageFreight drives it.

**Core capabilities:**
- **Dockerfile-native** — standard Dockerfiles are the build definition. StageFreight never generates or rewrites Dockerfiles. Your Dockerfile is your Dockerfile.
- **Multistage build support** — first-class support for multistage Dockerfiles via `target` selection:
  - **Default:** builds the final stage (standard Docker behavior)
  - **Named targets:** `target: production` to build a specific stage — useful when the same Dockerfile has `dev`, `test`, and `production` stages
  - **Multiple targets from one Dockerfile:** define multiple build configs that each target a different stage, producing separate images from a single Dockerfile (e.g., `app` image from the `production` stage, `migrations` image from the `migrate` stage, `worker` image from the `worker` stage)
  - **Dev target:** `stagefreight dev build` can target a `dev` stage that includes debug tools, live-reload entrypoints, or relaxed permissions — while CI targets the `production` stage. Same Dockerfile, different output.
- **Multi-registry push** — build once, push to N registries in a single invocation. Each registry gets its own image name/path — no assumption that they match.
- **Multi-arch builds** — `linux/amd64`, `linux/arm64`, `linux/arm/v7`, etc. Builds all platforms and assembles OCI image indexes (manifest lists) automatically.
- **Multi-OS builds** — `linux`, `windows`, `freebsd` where base images support it. Rarely used but some projects need it (e.g., Windows Server Core variants).
- **Flexible tag strategies** — per-registry tag templates with variables: `{version}`, `{major}`, `{major}.{minor}`, `{branch}`, `{sha}`, `{date}`, `latest`. Each registry can have its own tag set. Supports vendor-specific conventions (e.g., LinuxServer.io uses `<version>-ls<build>` tags).
- **Registry-specific naming** — some registries expect `library/name`, others `org/name`, others `org/sub/name`. The config maps each freely.
- **Branch filtering** — control which branches trigger builds and which registries they push to. Main branch pushes to all registries, feature branches push only to dev registry, tags push release tags everywhere.
- **Scheduled/continuous rebuilds** — daemon mode can trigger rebuilds at configured intervals (daily, weekly, on-upstream-change) to pick up base image security patches without waiting for a code change. Frequency is per-repo configurable.
- **Build context detection** — auto-detect Dockerfile location, build args, target stages. Override everything in config.
- **Local dev mode** (`--local`) — builds for the current platform only, loads into local Docker daemon, skips registry push. A dev runs `stagefreight docker build --local` and gets the exact same image CI would produce, tagged for local use. No registry credentials needed.

**Semantic cache invalidation:**

Docker layer caching is fast but dumb. It tracks file changes per `COPY`/`ADD` instruction, not what those files *mean* to the build. Compiled languages with asset embedding create hidden dependencies that layer caching gets wrong:

| Pattern | Problem | What StageFreight does |
|---|---|---|
| Go `//go:embed static/*` | Frontend assets baked into the binary at compile time. CSS/JS changes don't invalidate the Go build layer — stale UI ships. | Parses `embed` directives, tracks referenced paths as cache keys for the Go build layer. Asset change → Go layer rebuilds. |
| Rust `include_str!("schema.sql")` | Compile-time file inclusion. Schema changes don't bust the Rust build cache. | Parses `include_str!`/`include_bytes!` macros, same treatment. |
| Webpack/Vite bundle → `COPY dist/ .` | Bundle is built in an earlier stage, copied into a later stage. If the bundle stage is cached but source changed, stale bundle ships. | Tracks stage dependencies — if stage A's inputs changed, stages that `COPY --from=A` invalidate too. |
| `ARG VERSION` baked into binary | Version string compiled into the binary via ldflags or build arg. Layer cache ignores build arg changes by default. | Treats build args as cache keys — different `VERSION` value → rebuild. |
| Generated code (`protoc`, `sqlc`, `go generate`) | `.proto` files change but generated `.go` files are committed and cached. Build uses stale generated code. | Optional `cache.watch` paths — changes to watched files invalidate specified layers regardless of what Docker thinks. |

This isn't a new build system. StageFreight doesn't replace buildx caching — it wraps it with a semantic layer that understands cross-layer and cross-file dependencies. When StageFreight detects that a cached layer would produce stale output, it injects `--no-cache-filter` for that specific stage. Everything else stays cached.

```yaml
docker:
  cache:
    # Explicit cache-busting rules for cross-layer dependencies
    watch:
      - paths: ["web/src/**", "web/public/**"]    # if frontend source changes...
        invalidates: [build-go]                    # ...rebuild the Go stage that embeds it
      - paths: ["proto/**"]                        # if proto files change...
        invalidates: [generate, build]             # ...regenerate and rebuild
    # Auto-detection (default: true) — StageFreight scans for embed directives,
    # include macros, and COPY --from references without manual config
    auto_detect: true
```

Most projects never write `cache:` config. Auto-detection handles the common cases (Go embed, Rust includes, multi-stage COPY). The explicit `watch` rules exist for project-specific patterns that auto-detection can't infer — custom code generation, templating engines, or config files that change build output without changing source.

**Manifest config for builds:**
```yaml
docker:
  # Build configuration
  context: .                        # build context path
  dockerfile: Dockerfile            # Dockerfile path (auto-detected if omitted)
  target: production                # multistage target (omit for final stage)
  platforms:                        # multi-arch/multi-OS targets
    - linux/amd64
    - linux/arm64
    - linux/arm/v7
  build_args:
    APP_VERSION: "{version}"

  # Registry targets — each registry is fully independent
  registries:
    - url: docker.io
      path: prplanit/stagefreight
      tags:
        - "{version}"               # e.g., 1.2.3
        - "{major}.{minor}"         # e.g., 1.2
        - "{major}"                 # e.g., 1
        - latest
      branches: [main, master]      # only push from these branches
      credentials: DOCKERHUB_TOKEN

    - url: ghcr.io
      path: sofmeright/stagefreight
      tags:
        - "{version}"
        - latest
      branches: [main]
      credentials: GITHUB_TOKEN

    - url: lscr.io
      path: sofmeright/stagefreight
      tags:
        - "{version}-ls{build}"     # LinuxServer.io convention
        - latest
      branches: [main]
      credentials: LSCR_TOKEN

    - url: registry.prplanit.com
      path: tools/stagefreight
      tags:
        - "{version}"
        - "{branch}-{sha:.7}"       # feature branch builds: dev-abc1234
        - "{branch}-latest"
      branches: ["*"]               # all branches push here
      credentials: GITLAB_TOKEN

  # Scheduled rebuilds (daemon mode)
  rebuild:
    schedule: weekly                 # daily | weekly | monthly | cron expression
    on_base_update: true             # rebuild when base image has new digest
    notify: true                     # alert on rebuild (via configured alert channels)
```

**Multiple images from one Dockerfile (multistage):**

For simple projects (one Dockerfile → one image), use `docker.registries` directly as shown above. For projects that produce multiple distinct images from a single Dockerfile, use `docker.images[]` instead — each entry is a full build config with its own target, registries, tags, and platforms. The two styles are mutually exclusive: use `registries` for single-image, `images` for multi-image.

```yaml
docker:
  dockerfile: Dockerfile
  platforms: [linux/amd64, linux/arm64]    # shared default, images can override

  images:
    # Main application — targets the "production" stage
    - name: app
      target: production
      registries:
        - url: docker.io
          path: prplanit/my-app
          tags: ["{version}", latest]
        - url: ghcr.io
          path: sofmeright/my-app
          tags: ["{version}", latest]

    # Database migrations — targets the "migrate" stage
    # Ships as a separate image, runs as an init container or Job
    - name: migrations
      target: migrate
      registries:
        - url: docker.io
          path: prplanit/my-app-migrations
          tags: ["{version}"]

    # Background worker — targets the "worker" stage
    - name: worker
      target: worker
      platforms: [linux/amd64]             # override: workers only run on amd64
      registries:
        - url: docker.io
          path: prplanit/my-app-worker
          tags: ["{version}", latest]
```

The corresponding Dockerfile:
```dockerfile
# ---- shared base ----
FROM node:22-alpine AS base
WORKDIR /app
COPY package*.json ./
RUN npm ci --production

# ---- migrations image ----
FROM base AS migrate
COPY migrations/ ./migrations/
ENTRYPOINT ["npm", "run", "migrate"]

# ---- worker image ----
FROM base AS worker
COPY src/worker/ ./src/worker/
ENTRYPOINT ["npm", "run", "worker"]

# ---- production image ----
FROM base AS production
COPY src/ ./src/
EXPOSE 8080
ENTRYPOINT ["npm", "start"]

# ---- dev image (never pushed, used by stagefreight dev build) ----
FROM production AS dev
RUN npm install --include=dev
COPY tests/ ./tests/
ENTRYPOINT ["npm", "run", "dev"]
```

`stagefreight docker build` builds all images in `docker.images`, sharing layer cache across targets. `stagefreight dev build` automatically targets the `dev` stage if one exists (configurable via `dev.target`), giving developers debug tools and test dependencies without bloating the production image.

#### Artifact Builds & Multi-Toolchain Pipelines

Not every build produces a container image. Some produce binaries, zips, installers, or tarballs. Some need multiple toolchains in sequence (Go + .NET, Rust + C, etc.). Some cross-compile for targets where containers don't apply (Windows `.exe`, macOS `.app`, embedded firmware).

Today these live as raw CI scripts — installing dependencies at runtime, fragile, slow, not reproducible locally. The entire point of StageFreight is to make these declarative and reproducible.

**`stagefreight build`** (not `docker build` — this is the general-purpose build command)

The key insight: **use Docker as the build environment even when the output isn't a Docker image.** A Dockerfile defines the reproducible toolchain, multistage builds compile the artifacts, and StageFreight extracts the outputs — binaries, zips, packages — from the final stage. The developer runs the same command locally and gets the same artifacts CI produces.

**Manifest config for artifact builds:**
```yaml
build:
  # Artifact builds — output is files, not container images
  artifacts:
    # Example: Beszel agent for Windows (Go + .NET cross-compile)
    - name: beszel-agent-windows
      dockerfile: build/Dockerfile.beszel-windows
      target: export                   # stage that holds the final artifacts
      platforms: [linux/amd64]         # build platform (cross-compiles TO windows inside)
      extract:                         # pull files OUT of the build container
        - from: /out/beszel-agent.exe
          to: dist/beszel-agent-windows/
        - from: /out/*.dll
          to: dist/beszel-agent-windows/
      package:
        - type: zip
          name: "beszel-agent-windows-{version}.zip"
          contents: dist/beszel-agent-windows/
      build_args:
        UPSTREAM_VERSION: "{version}"

    # Example: CLI tool for multiple OS/arch combos
    - name: my-cli
      dockerfile: build/Dockerfile.cli
      matrix:                          # build a matrix of targets
        - { goos: linux,   goarch: amd64, suffix: linux-amd64 }
        - { goos: linux,   goarch: arm64, suffix: linux-arm64 }
        - { goos: darwin,  goarch: amd64, suffix: darwin-amd64 }
        - { goos: darwin,  goarch: arm64, suffix: darwin-arm64 }
        - { goos: windows, goarch: amd64, suffix: windows-amd64, ext: .exe }
      extract:
        - from: "/out/my-cli{ext}"
          to: "dist/my-cli-{suffix}/"
      package:
        - type: tar.gz
          name: "my-cli-{version}-{suffix}.tar.gz"
          contents: "dist/my-cli-{suffix}/"
          if: "suffix != 'windows-amd64'"    # tar.gz for unix
        - type: zip
          name: "my-cli-{version}-{suffix}.zip"
          contents: "dist/my-cli-{suffix}/"
          if: "suffix == 'windows-amd64'"    # zip for windows

  # Where to upload release artifacts
  release_assets: true               # attach packages to the git release (via provider API)
```

The Beszel Windows example as a Dockerfile instead of a CI script:
```dockerfile
# ---- .NET SDK for LibreHardwareMonitor ----
FROM mcr.microsoft.com/dotnet/sdk:8.0 AS dotnet-build
WORKDIR /src
ARG UPSTREAM_VERSION
RUN git clone --depth 1 --branch ${UPSTREAM_VERSION} https://github.com/henrygd/beszel.git
WORKDIR /src/beszel/beszel
RUN dotnet build -c Release ./internal/agent/lhm/beszel_lhm.csproj

# ---- Go build with LHM DLLs ----
FROM golang:1.24-bullseye AS go-build
WORKDIR /src
COPY --from=dotnet-build /src/beszel /src/beszel
WORKDIR /src/beszel/beszel
RUN go mod tidy
WORKDIR /src/beszel/beszel/cmd/agent
RUN GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-w -s" -o /out/beszel-agent.exe

# ---- Collect all artifacts ----
FROM scratch AS export
COPY --from=go-build /out/beszel-agent.exe /out/
COPY --from=dotnet-build /src/beszel/beszel/internal/agent/lhm/bin/Release/net48/*.dll /out/
```

What was a 50-line CI script with `apt-get install` and `wget` is now a declarative, cached, reproducible Dockerfile. A developer runs `stagefreight build` locally and gets the same `beszel-agent-windows-v0.9.1.zip` that CI produces.

**How `stagefreight build` works:**
1. Reads `build.artifacts[]` from the manifest
2. For each artifact (or matrix expansion), runs `docker buildx build` targeting the specified stage
3. Uses `--output type=local` to extract files from the build container to the local filesystem
4. Runs `package` steps (zip, tar.gz, etc.) on extracted files
5. If `release_assets: true`, uploads packages to the git release via the provider API
6. All of this works identically for `stagefreight dev build` — devs get the same artifacts locally

**Matrix builds** expand a single artifact definition across multiple target combinations. Each matrix entry injects its variables as build args and template values. The build runs in parallel where possible (buildx cache sharing across matrix entries).

**Relationship to `docker build`:**
- `stagefreight docker build` — output is container images pushed to registries
- `stagefreight build` — output is files extracted from build containers, optionally packaged and attached to releases
- Both use Dockerfiles as the build definition. Both are reproducible locally. Both benefit from buildx caching. The only difference is what comes out.

#### `stagefreight dev` — Local Developer Experience

The entire point of codifying builds and tests in the manifest is that **every environment runs the same thing**. A dev on their laptop, CI in a pipeline, and the daemon in production all read the same `.stagefreight.yml` and execute the same logic. No "works on my machine" — the manifest IS the machine.

**`stagefreight dev build`**
- Detects the project type and runs the right build:
  - **If `docker:` config exists** (or Dockerfile found): builds a container image for the current platform, loads into local Docker daemon. Equivalent to `stagefreight docker build --local`.
  - **If `build.artifacts:` config exists**: builds artifacts for the current platform, extracts to `dist/`. Equivalent to `stagefreight build` scoped to the local OS/arch.
  - **If both exist**: builds both. Container images load locally, artifacts extract to disk.
- Tags images as `<app>:dev` (or `<app>:<branch>-dev` if on a feature branch)
- **Targets the `dev` stage** if the Dockerfile has one (configurable via `dev.target`). The dev stage typically extends the production stage with debug tools, test dependencies, and hot-reload entrypoints — stuff that never ships to production but makes local development ergonomic.
- Same Dockerfile, same build args as CI — just a different target stage and single platform
- No registry push, no credentials needed
- Fast iteration: rebuilds use layer cache, only changed layers rebuild

**`stagefreight dev test`**
- Spins up the locally built image using the `dev.test` config from the manifest
- Injects dev-mode environment variables, mounts, and test endpoints
- Runs health checks and sanity tests the developer has defined
- Reports pass/fail with clear output — same checks CI runs, same checks the daemon runs
- Tears down cleanly after tests complete (or keeps running with `--keep` for manual poking)

**`stagefreight dev up`**
- Brings up the full dev environment defined in the manifest (app + dependencies)
- Supports compose-style service dependencies (database, cache, etc.) via the `dev.services` block
- Wires up networking, volumes, and env vars per manifest config
- Stays running until `ctrl-c` or `stagefreight dev down`

**The dev workflow:**
```
$ cd my-project
$ stagefreight dev build        # build image for my platform
$ stagefreight dev test         # run health checks + sanity tests
$ stagefreight dev up           # spin up app + deps for manual testing
```

All three commands read the same `.stagefreight.yml`. A contributor clones the repo, installs the `stagefreight` binary, and has a working dev environment with validated tests — no README scavenger hunt for "how do I run this locally."

**Manifest config for dev/test:**
```yaml
dev:
  # Dockerfile stage to target for dev builds (auto-detected if "dev" stage exists)
  target: dev

  # Environment for local testing (overrides for dev context)
  env:
    DATABASE_URL: postgres://dev:dev@db:5432/app_dev
    REDIS_URL: redis://cache:6379
    LOG_LEVEL: debug
    SECRET_KEY: dev-secret-not-for-production

  # Service dependencies (spun up by `stagefreight dev up`)
  services:
    db:
      image: docker.io/library/postgres:17-alpine
      env:
        POSTGRES_USER: dev
        POSTGRES_PASSWORD: dev
        POSTGRES_DB: app_dev
      ports: [5432]
      healthcheck:
        test: pg_isready -U dev
        interval: 5s
        retries: 3

    cache:
      image: docker.io/library/redis:7-alpine
      ports: [6379]

  # Ports to expose from the app container during dev up
  ports:
    - "8080:8080"
    - "9090:9090"                  # metrics/debug port

  # Volume mounts for live reload / persistent dev data
  volumes:
    - ./src:/app/src               # live code mount for hot reload
    - dev-data:/app/data           # persistent dev data across restarts

  # Test suite — runs against the built image
  test:
    # Startup: bring up the app (and services if needed) before testing
    startup:
      timeout: 30s                 # max time to wait for app to be ready
      depends_on: [db, cache]      # ensure deps are healthy first

    # Health checks — HTTP/TCP probes that must pass
    healthchecks:
      - name: http-ready
        type: http
        url: http://localhost:8080/health
        expect_status: 200
        timeout: 5s

      - name: metrics-endpoint
        type: http
        url: http://localhost:9090/metrics
        expect_status: 200
        expect_body_contains: "app_requests_total"

      - name: db-connected
        type: tcp
        host: localhost
        port: 5432
        timeout: 3s

    # Sanity checks — custom commands run inside the app container
    # Devs package whatever validation they want here
    sanity:
      - name: migrations-current
        run: ./manage.py migrate --check
        description: "Verify all migrations are applied"

      - name: seed-data-loads
        run: ./manage.py loaddata testfixtures
        description: "Verify test fixtures load cleanly"

      - name: api-smoke-test
        run: curl -sf http://localhost:8080/api/v1/status | jq -e '.ok == true'
        description: "Basic API response validation"

      - name: unit-tests
        run: pytest tests/ -x --tb=short
        description: "Run unit test suite"

    # Exit behavior
    teardown: always               # always | on-success | never
```

**What `stagefreight dev test` actually does:**
1. Builds the image if not already built (or uses last `dev build`)
2. Starts service dependencies, waits for their healthchecks
3. Starts the app container with `dev.env` injected
4. Waits for `startup.timeout`, checking app healthchecks
5. Runs `test.healthchecks` — HTTP/TCP probes with expected results
6. Runs `test.sanity` — custom commands inside the container, in order, stops on first failure (or `--continue` to run all)
7. Reports results: pass/fail per check with timing
8. Tears down per `teardown` policy

**CI parity:** In a pipeline, `stagefreight dev test` runs identically — same manifest, same checks, same pass/fail criteria. The only difference is CI pushes the image to registries afterward if tests pass. The daemon uses the same test suite for scheduled rebuild validation before pushing patched images.

#### Pipeline-First Philosophy

StageFreight works in two modes: **binary tool** (developer runs `stagefreight` on their laptop) and **pipeline** (CI/CD runs `stagefreight` in a controlled environment). Both execute the same manifest. Both produce the same output. But the pipeline is the golden path — and the experience should be so smooth that developers naturally prefer pushing a commit over building locally.

**Why pipelines win:**

| | Local Dev | Pipeline |
|---|---|---|
| **Secrets** | Developer needs registry credentials, API tokens, signing keys on their machine. Every laptop is an attack surface. | Secrets live in CI variables, never touch a developer's filesystem. Rotated centrally, audited, scoped per-branch. |
| **Environment** | "Works on my machine" — different Docker versions, OS quirks, stale caches, disk pressure, VPN issues. | Identical runner image every time. Clean environment, known state, no accumulated cruft. |
| **Multi-arch** | QEMU emulation is slow and painful. Most devs skip arm64 builds locally and hope CI catches it. | Runners can be native per-arch, or QEMU on dedicated build nodes. Cross-compilation happens once, correctly, without developer pain. |
| **Cache** | Local Docker cache is per-machine, diverges across team members, hard to share. | Pipeline cache is shared across runs. StageFreight's semantic cache invalidation works best with a persistent, centralized cache store. |
| **Security** | Lint and security scans run with the developer's permissions. A compromised dependency can access anything the developer can. | Sandboxed runner. Network-restricted. No access to production systems. Scan results are artifacts, not local files. |
| **Audit trail** | Nothing. No record of what was built, when, by whom, from what commit. | Every build is a pipeline run with commit SHA, manifest version, lint results, scan output, SBOM, and push receipt. Auditors love this. |

**The developer experience should make this feel obvious:**

```
$ git push origin feature/new-auth
  → Pipeline starts automatically
  → lint ✓ (0.3s, cached)
  → build ✓ linux/amd64,arm64 (12s, layer cache hit)
  → test ✓ healthchecks + 4 sanity checks (8s)
  → scan ✓ 0 critical, 2 low
  → push ✓ registry.prplanit.com/apps/myapp:feature-new-auth-abc1234
  → MR comment: "Build passed. Image ready at registry.prplanit.com/apps/myapp:feature-new-auth-abc1234"
```

A developer pushes, gets coffee, comes back to a green pipeline with a tested, scanned, tagged image waiting in the registry. No local Docker daemon needed. No credentials on their laptop. No QEMU. The pipeline did everything faster and more securely than they could have locally.

**Local dev still works and still matters.** You need it for interactive debugging, stepping through code, testing UI changes in real time. `stagefreight dev build` and `stagefreight dev up` exist specifically for this. But the feedback loop for "does my code pass all checks and produce a correct image" should be: push → pipeline → done. Local builds are for iteration. Pipelines are for validation.

**How StageFreight makes the pipeline feel effortless:**
- **Zero CI config authoring** — the GitLab CI component (or GitHub Actions wrapper) reads the `.stagefreight.yml` manifest. The pipeline definition is one line: `include: stagefreight`. No writing CI YAML by hand.
- **Same command everywhere** — `stagefreight docker build` in a pipeline is the same command a dev runs locally. No "CI does something different." If it passes locally, it passes in CI. If it fails in CI, you can reproduce locally with the same command.
- **Pipeline-aware caching** — StageFreight persists its lint cache, buildx cache, and scan results as CI artifacts. Subsequent pipeline runs on the same branch pick up where the last one left off. Feature branch builds benefit from main branch cache.
- **Branch-specific registries** — feature branches push to the dev registry with `{branch}-{sha:.7}` tags. Main branch pushes to all production registries. Developers see their feature branch image in the registry within minutes of pushing, without polluting production tags.
- **Inline MR feedback** — scan results, lint failures, and test output appear as MR comments. The developer never leaves the MR page to understand what happened.

The pipeline should feel like an extension of the developer's editor — push, and the rest happens. Local dev is the escape hatch for when you need hands-on iteration. The pipeline is where real builds happen.

**Deliverables:**
- Go module with subcommand structure (cobra or similar)
- Binary embedded in OCI image alongside existing tools
- Updated GitLab CI component templates calling the binary instead of scripts
- Existing bash scripts remain as fallback until all parity is confirmed

### Phase 1 — Multi-Provider

Extract the GitLab-specific logic into a provider interface and add GitHub + Gitea/Forgejo support.

```go
type Provider interface {
    Repos(ctx context.Context, org string) ([]Repo, error)
    CreateMR(ctx context.Context, repo Repo, opts MROptions) (*MR, error)
    CommitFile(ctx context.Context, repo Repo, path string, content []byte, msg string) error
    CreateRelease(ctx context.Context, repo Repo, opts ReleaseOptions) (*Release, error)
    PipelineStatus(ctx context.Context, repo Repo, ref string) (Status, error)
    // ...
}
```

| Provider | Library | Notes |
|---|---|---|
| GitLab | `go-gitlab` | Existing logic extracted from bash, first-class support |
| GitHub | `go-github` | Second provider, enables cross-platform CLI usage |
| Gitea/Forgejo | `go-sdk` | Covers self-hosted Git forges |

**Deliverables:**
- Provider interface + three implementations
- CLI auto-detects provider from git remote or `--provider` flag
- GitHub Actions wrapper (equivalent of the GitLab CI component)

### Phase 2 — Cross-Provider Sync

New subcommands for keeping repo metadata consistent across providers and registries.

| Subcommand | Description |
|---|---|
| `stagefreight sync readme` | Push README to DockerHub, GHCR, Quay descriptions |
| `stagefreight sync metadata` | Mirror description, topics, homepage URLs across providers |
| `stagefreight lint links` | Crawl README/docs for broken links, report or auto-fix |
| `stagefreight flair` | Auto-manage shields.io badges in README |

Nothing good exists for cross-provider doc sync today. DockerHub readme sync has a few janky GitHub Actions but nothing unified. This is greenfield.

### Phase 3 — Fork Maintenance

**Unique differentiator — nothing like this exists.**

Keeping a patched fork current with upstream while preserving your changes and scanning for safety. People do this manually and it sucks.

| Subcommand | Description |
|---|---|
| `stagefreight fork sync` | Rebase or merge upstream changes, preserve local patches |
| `stagefreight fork status` | Divergence report — commits ahead/behind, conflict detection |
| `stagefreight fork audit` | Scan upstream changes for suspicious commits (new maintainers, obfuscation, unexpected binary additions, invisible characters, AI prompt injection payloads) |

**Design considerations:**
- `strategy: rebase` vs `strategy: merge` per-repo config
- Conflict detection creates an MR with conflict markers for human resolution
- Audit heuristics flag but never block — advisory only
- `fork audit` integrates with `audit codebase` — every upstream sync runs invisible character detection on the incoming diff, not just the full codebase. Catches Trojan Source and prompt injection payloads injected by compromised upstream maintainers.

### Phase 4 — Supply Chain & Security

Extends the existing `security scan` subcommand with pre-build dependency auditing and SBOM tracking.

| Subcommand | Description |
|---|---|
| `stagefreight audit deps` | Pre-build dependency auditing — scan lockfiles for known-bad versions before building |
| `stagefreight audit packages` | Cross-reference dependencies against known package-level attacks (typosquatting, maintainer takeovers) |
| `stagefreight audit codebase` | Scan source files for invisible/malicious character sequences (see below) |
| `stagefreight sbom diff` | Track SBOM changes between releases — what was added, removed, or changed |

#### `stagefreight audit codebase` — Invisible Character & Injection Scanning

> **Note:** The detection engine described here is the same one that powers `stagefreight lint --module unicode`. Lint runs it on changed files during every build; `audit codebase` runs it against the entire repository as a deep sweep. Use `audit codebase` for initial adoption audits, periodic full-repo sweeps, and fork sync verification. Use `lint` for the per-commit guard.

Source code is text, and text can lie. Characters exist that are invisible to humans but interpreted by compilers, rendered by terminals, or consumed by AI assistants. This is a real and growing attack surface — especially as AI coding tools read every file in a repo.

**What it detects:**

| Category | Characters / Patterns | Attack |
|---|---|---|
| **Trojan Source** (CVE-2021-42574) | Unicode bidi overrides: `U+202A` (LRE), `U+202B` (RLE), `U+202C` (PDF), `U+202D` (LRO), `U+202E` (RLO), `U+2066` (LRI), `U+2067` (RLI), `U+2068` (FSI), `U+2069` (PDI) | Code appears different in editors than what compilers execute. Reorders characters visually to hide malicious logic in plain sight. |
| **Zero-width characters** | `U+200B` (zero-width space), `U+200C` (ZWNJ), `U+200D` (ZWJ), `U+FEFF` (BOM mid-file), `U+2060` (word joiner), `U+180E` (Mongolian vowel separator) | Invisible characters that break string comparisons, hide payloads in identifiers, or smuggle data through copy-paste. Two variable names can look identical but be different. |
| **Homoglyph attacks** | Cyrillic `а` vs Latin `a`, Greek `ο` vs Latin `o`, and hundreds of similar pairs | Identifiers, URLs, or strings that look correct but reference different things. `pаypal.com` (Cyrillic а) vs `paypal.com`. |
| **ANSI escape sequences** | `\x1b[`, `\033[`, `\e[` in non-terminal contexts (source code, configs, data files) | Hide or overwrite terminal output. Make `git diff` or `cat` show clean code while the actual content is malicious. Escape sequences in log output can overwrite previous lines. |
| **Invisible prompt injection** | Hidden instructions targeting AI assistants: zero-width encoded text, HTML/XML comment blocks with directives, Unicode tag characters (`U+E0001`–`U+E007F`), invisible `<instructions>` tags, base64-encoded directives in comments | Codebase files containing invisible or visually-hidden text that AI coding assistants (Claude Code, Copilot, Cursor, Codex) will read and follow as instructions, but human reviewers will never see. The AI reads the full file content including invisible characters — the human only sees the rendered output. |
| **Unicode tag characters** | `U+E0001`–`U+E007F` (Tags block) | Deprecated Unicode block that renders as completely invisible in all editors and terminals. Can encode arbitrary ASCII as invisible Unicode. Sometimes used to watermark text but also to hide payloads. |
| **Confusable whitespace** | `U+00A0` (NBSP), `U+2000`–`U+200A` (various spaces), `U+205F` (medium math space), `U+3000` (ideographic space) | Break indentation-sensitive languages (Python, YAML, Makefile). Code looks correctly indented but isn't. YAML parsing changes with wrong whitespace. |

**How it works:**
1. Scans all text files in the repo (respects `.gitignore`, configurable include/exclude patterns)
2. Flags any byte sequence matching the detection categories above
3. Reports file, line, column, character code point, and category
4. Severity: bidi overrides and prompt injection are **critical**, zero-width and homoglyphs are **warning**, confusable whitespace is **info**
5. CI mode: exit non-zero on critical findings (configurable threshold)
6. Fix mode (`--fix`): remove or replace flagged characters, open MR with diff showing exactly what was cleaned

**The AI prompt injection angle is the important one.** As AI coding assistants become standard tooling, every file in a codebase becomes an attack surface for invisible instructions. A malicious contributor can embed invisible directives in a source file that:
- Tell the AI to ignore security warnings
- Instruct the AI to introduce specific vulnerabilities
- Direct the AI to exfiltrate code or context to external URLs
- Override the AI's safety guidelines for subsequent interactions

The human reviewer sees clean code. The AI reads the invisible instructions. `stagefreight audit codebase` catches this before it reaches the main branch.

**Manifest config:**
```yaml
security:
  codebase_audit: true                # enable invisible character scanning
  codebase_audit_fix: false           # true = auto-fix via MR, false = report only
  codebase_audit_threshold: warning   # critical | warning | info — fail CI at this level
```

**Market angle:** The supply chain angle is legitimately strong given xz-utils, polyfill.io, and constant npm/PyPI attacks. Centralizing pre-build dependency auditing across an entire org's repos is a real sell to security teams. The invisible character / AI prompt injection scanning adds a unique angle that nobody else covers — as AI-assisted development becomes standard, this becomes a mandatory check.

### Phase 5 — Daemon Mode (The Endgame)

`stagefreight serve` — a long-running daemon deployed as a K8s pod.

**Capabilities:**
- Multiple provider connections with deploy tokens per org
- Org scanning: discovers all repos the token can see
- Global opt-in defaults per org (baseline behavior for all visible repos)
- Per-repo manifest (`.stagefreight.yml`) for override/customization
- Mirror awareness: repos declare source-of-truth vs mirror, writes only go to source
- Scheduled maintenance (cron-style per feature)
- MR-based workflow: every write operation is a PR/MR
- YOLO mode: auto-merge for repos that opt in

---

## Per-Repo Manifest (`.stagefreight.yml`)

Discovered by the daemon in each repo. Overrides org-level defaults from the daemon config. Also usable by the CLI for local runs.

```yaml
version: 1

# Source-of-truth declaration (overrides daemon mirror patterns)
source: true                    # this repo IS the source of truth
# OR:
# mirror_of: gitlab.prplanit.com/precisionplanit/my-project  # this is a mirror

release:
  notes: true
  badge: true

docker:
  platforms: [linux/amd64, linux/arm64]
  registries:
    - url: docker.io
      path: prplanit/my-app
      tags: ["{version}", "{major}.{minor}", latest]
    - url: ghcr.io
      path: sofmeright/my-app
      tags: ["{version}", latest]
  rebuild:
    schedule: weekly
    on_base_update: true

lint:
  level: changed                  # full | changed (default) | off
  modules:
    secrets: true
    unicode: true
    yaml: true
    tabs: false                   # project uses tabs — disable this module
  exclude: ["vendor/**"]

docs:
  readme_inputs: true
  readme_sync: [dockerhub]      # push README to these registry descriptions

security:
  scan: true
  sbom: true
  fail_on_critical: false
  audit_deps: true
  secret_findings: alert          # alert (default, discrete) | mr (opt-in MR mode)
  codebase_audit: true            # invisible character + AI prompt injection scanning
  codebase_audit_threshold: warning   # critical | warning | info

license:
  compliance: true
  headers: true                   # enforce SPDX headers in source files
  spdx: true                     # generate SPDX documents

templates:
  golden_files: true              # sync .editorconfig, CONTRIBUTING.md, etc. from org template
  gitignore: true                 # lint and fix .gitignore for detected languages

fork:
  upstream: github.com/original/repo
  strategy: rebase              # or: merge
  auto_sync: true

dev:
  target: dev                 # Dockerfile stage for dev builds (auto-detected if "dev" stage exists)
  env:
    DATABASE_URL: postgres://dev:dev@db:5432/app_dev
    LOG_LEVEL: debug
  services:
    db:
      image: docker.io/library/postgres:17-alpine
      env: { POSTGRES_USER: dev, POSTGRES_PASSWORD: dev, POSTGRES_DB: app_dev }
      healthcheck: { test: "pg_isready -U dev", interval: 5s }
  ports: ["8080:8080"]
  test:
    startup: { timeout: 30s, depends_on: [db] }
    healthchecks:
      - { name: http-ready, type: http, url: "http://localhost:8080/health", expect_status: 200 }
    sanity:
      - { name: smoke-test, run: "curl -sf http://localhost:8080/api/status | jq -e '.ok'" }
      - { name: unit-tests, run: "pytest tests/ -x" }

flair:
  badges: auto                  # auto-manage shields.io badges in README

yolo: false                     # auto-merge MRs opened by stagefreight
```

**Resolution order:** Per-repo `.stagefreight.yml` > daemon org defaults > nothing (repo ignored if no manifest and no global opt-in).

---

## Daemon Config

What the `stagefreight serve` pod reads on startup. Defines provider connections, org-level defaults, scan intervals, and mirror rules.

```yaml
providers:
  - name: gitlab-prplanit
    type: gitlab
    url: https://gitlab.prplanit.com
    token: ${GITLAB_TOKEN}
    orgs: [precisionplanit, components]
    defaults:
      release: { notes: true, badge: true }
      security: { scan: true, secret_findings: alert }
      license: { compliance: true }
      templates: { golden_files: true, gitignore: true }

  - name: github-sofmeright
    type: github
    token: ${GITHUB_TOKEN}
    orgs: [sofmeright]
    defaults:
      release: { notes: true }
      security: { secret_findings: alert }

alerts:
  # Where discrete findings (secrets, critical CVEs) are sent
  - type: smtp
    to: security@precisionplanit.com
  - type: webhook
    url: ${ALERT_WEBHOOK_URL}

scan:
  interval: 15m
```

### Mirror Rules

Mirror rules define automated git push relationships between providers. The daemon evaluates these on every scan cycle.

```yaml
mirrors:
  # Mirror a group, flattening the path
  - source:
      provider: gitlab-prplanit
      match: "precisionplanit/.*"
      ignore: "precisionplanit/private-.*"
    target:
      provider: github-sofmeright
      path: "sofmeright/{name}"

  # Mirror components, flattening group structure
  - source:
      provider: gitlab-prplanit
      match: "components/.*"
    target:
      provider: github-sofmeright
      path: "sofmeright/{name}"

  # Broad match — everything readable, preserving structure
  - source:
      provider: gitlab-prplanit
      match: ".*"
      ignore: "(private-|internal/).*"
    target:
      provider: github-sofmeright
      path: "{group}/{name}"
```

**Template variables:** `{domain}`, `{group}`, `{name}`, `{path}` (full group/name).

**Mirror engine behavior:**
- Evaluates `match` regex against `{group}/{name}` of every discovered repo
- Skips repos matching `ignore` regex
- Applies `path` template to determine target repo location
- **Auto-push**: if a rule matches and the daemon has write access to the target, it pushes — the mirror config IS the opt-in
- Source-of-truth is always the `source` side — writes/MRs only go to source, git pushes go to target
- Per-repo `.stagefreight.yml` can override with `source: true` or `mirror_of:` to declare relationships explicitly
- A repo declaring `mirror_of:` in its manifest will never be treated as a source, even if a mirror rule would match it as one

---

## Planned Features (Beyond Core Phases)

Features accepted for implementation, to be slotted into phases as the core architecture matures.

### Template & Boilerplate Sync

| Feature | Description |
|---|---|
| **Golden file management** | Define template files (`.editorconfig`, `.gitattributes`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, `LICENSE`) in a template repo or daemon config. StageFreight ensures all repos in the org stay in sync — opens MRs when drift is detected. |
| **CI template drift** | When a shared CI template or reusable workflow is updated, detect which repos are still on the old version and open upgrade MRs. |
| **`.gitignore` drift** | Detect repos missing standard ignores for their detected language/framework. Lint and fix via MR. |

### License Management

| Feature | Description |
|---|---|
| **License compliance scanning** | Scan all repos' dependencies for license compatibility. Flag AGPL pulled into MIT projects, GPL in proprietary codebases, etc. |
| **License header enforcement** | Ensure source files have proper SPDX headers. Open MRs to add/fix them. |
| **SPDX/SBOM generation** | Full SPDX document generation per repo — extends Phase 4 SBOM work into a standalone capability. |

### Secrets & Credential Hygiene

| Feature | Description |
|---|---|
| **Org-wide secret scanning** | Sweep all repos the daemon can see for committed secrets (API keys, tokens, passwords, private keys). Unlike per-repo pre-commit hooks, this catches secrets that were committed before tooling was in place. |
| **CI variable staleness** | Detect expired tokens, references to deleted runners, or deprecated images in CI configs across the org. |

**Disclosure policy:** Discovered secrets are sensitive — StageFreight does NOT open MRs by default for secret findings. Instead:
- **Default (discrete mode):** Findings are logged to a configured alert channel (SMTP, webhook, Slack, syslog) visible only to repo owners/admins. The alert includes the file, line, secret type, and remediation steps — but never the secret value itself.
- **Opt-in MR mode:** Repos can set `security.secret_findings: mr` in `.stagefreight.yml` to have StageFreight open MRs that rotate/remove the secret. MRs are marked confidential where the provider supports it (GitLab confidential MRs, GitHub draft PRs with restricted visibility).
- **Never in public view:** Secret findings never appear in public issue trackers, commit messages, or non-confidential MRs regardless of configuration.

### Repository Hygiene

| Feature | Description |
|---|---|
| **Tag hygiene** | Detect inconsistent tag naming (`v1.0` vs `1.0` vs `release-1.0`), missing tags for releases, orphaned tags pointing at deleted branches. Normalize via MR or report. |
| **Stale issue/MR triage** | Auto-label, auto-close, or ping on issues/MRs that have gone cold. Cross-provider — works across GitLab, GitHub, Gitea simultaneously. Cross-repo sync for issues that span multiple repos. |

### Container Image Lifecycle

| Feature | Description |
|---|---|
| **Base image staleness** | Scan Dockerfiles across the org for pinned-to-old or EOL base images. Open upgrade MRs. |
| **Registry tag cleanup** | Enforce retention policies on container registries — delete tags older than N days, keep only the last N tags per branch. Registries balloon silently. |
| **Image provenance chain** | Full traceability from source commit → build → registry tag. Strengthens the supply chain story. |
| **Security patch rebuilds** | Monitor published images for base-layer CVEs. When a security patch lands upstream, automatically rebuild and republish affected releases from their known-good build assets or repeatable build configs. Covers old releases that maintainers forget to patch — if the original build is reproducible (Dockerfile + pinned deps), StageFreight rebuilds it with the patched base and pushes a point release. Driven by `docker.rebuild.on_base_update` in the build engine. |
| **Continuous rebuild scheduling** | Daemon triggers rebuilds at configured intervals (`daily`, `weekly`, `monthly`, cron expression) to pick up base image patches without waiting for code changes. Frequency is per-repo via `docker.rebuild.schedule`. |

### Release Automation

| Feature | Description |
|---|---|
| **Changelog maintenance** | Keep `CHANGELOG.md` as a living file updated on every merge, not just at release time. |
| **Semantic version enforcement** | Based on conventional commits or commit categories, enforce/suggest the next version number. |
| **Release notifications** | Webhook, Slack, or email on new releases. Natural fit for daemon mode since it already watches every repo. |
| **Breaking change propagation** | When a dependency (internal or external) releases a major version, open MRs across all affected repos in the org. |

### Documentation Quality

| Feature | Description |
|---|---|
| **README completeness scoring** | Score repos on whether they have description, install instructions, usage examples, license section, etc. Open MRs with skeletons for what's missing. The "repo lander advice" feature. |
| **Broken cross-references** | Detect rotted links between your own repos' docs — internal org links are a separate beast from external URL linting. |

---

## Low Priority / Backlog

Ideas worth tracking but not yet committed to. May be promoted to Planned as the tool matures.

| Feature | Description |
|---|---|
| **Stale branch cleanup** | Branches merged but never deleted, branches with no activity for N months. Open MRs to delete or just clean in YOLO mode. |
| **Internal dependency graph** | Map which of your repos depend on which of your other repos. Visualize the internal dependency web across heterogeneous language ecosystems. |

---

## Feature Matrix vs Existing Tools

| Feature | Existing tools (fragmented) | StageFreight |
|---|---|---|
| Readme badges/flair management | Shields.io (manual) | `flair` — auto-managed |
| Dependency updates | Renovate, Dependabot (standalone) | `audit deps` — pre-build auditing |
| Security scanning | Snyk, Socket, Trivy (standalone) | `security scan` + `audit packages` |
| Cross-provider doc sync | Nothing good exists | `sync readme` + `sync metadata` |
| Fork maintenance with patch preservation | **Nothing exists** | `fork sync` + `fork status` + `fork audit` |
| Pre-build supply chain sleuthing | Socket.dev is closest | `audit deps` + `audit packages` |
| DockerHub readme sync | A few janky Actions | `sync readme` |
| SBOM tracking across releases | Manual diffing | `sbom diff` |
| Auto-MR with YOLO auto-accept | **Nothing exists as a unified feature** | MR-first workflow + `yolo: true` |
| Multi-provider daemon with org scanning | **Nothing exists** | `stagefreight serve` |
| Golden file / boilerplate sync | Manual copy-paste across repos | Golden file management + CI template drift |
| License compliance across org | FOSSA, Snyk (expensive, standalone) | License compliance + header enforcement + SPDX |
| Org-wide secret scanning | GitGuardian, TruffleHog (standalone, noisy) | Discrete alert-first scanning with opt-in MRs |
| Base image CVE rebuilds | **Nothing exists** | Security patch rebuilds — auto-rebuild old releases |
| Registry tag retention | Manual or per-registry policies | Registry tag cleanup across all registries |
| Changelog maintenance | Release-drafter, changelogen (standalone) | Living `CHANGELOG.md` updated on every merge |
| README quality scoring | **Nothing exists** | README completeness scoring + skeleton MRs |
| Invisible character / Trojan Source detection | `grep` for bidi (manual, misses most) | `audit codebase` — full Unicode threat scanning |
| AI prompt injection in codebases | **Nothing exists** | `audit codebase` — invisible instruction detection |

---

## Market Positioning

**"The maintainer you wish you had."**

StageFreight does the tedious hygiene work that every repo needs but nobody wants to do. It replaces the patchwork of Renovate + Snyk + Trivy + manual badge maintenance + manual fork rebases with a single tool that understands your entire repo portfolio.

**Unique differentiators:**
- **Fork maintenance** — nothing like it exists. Keeping patched forks current with upstream while preserving changes and scanning for safety.
- **Cross-provider sync** — DockerHub readme, GitHub/GitLab mirror description parity, link verification.
- **Unified stewardship** — one tool, one manifest, one daemon watching everything.
- **Zero-config discovery** — daemon scans repos for opt-in manifests automatically, no per-repo registration.

**Supply chain angle:** xz-utils, polyfill.io, and constant npm/PyPI attacks make centralized pre-build dependency auditing across an entire org's repos a real sell to security teams.

**YOLO auto-accept** drives adoption from solo devs and homelab users fast. Low friction, high value.

---

## Branding

**StageFreight** absorbs the prplangit vision. The name is already established in CI pipelines via the GitLab component and OCI image. The stage fright + freight/shipping pun fits a tool that ships your code and manages the stage.

- **Image:** `docker.io/prplanit/stagefreight`
- **CLI:** `stagefreight`
- **Manifest:** `.stagefreight.yml`
- **Branch prefix:** `stagefreight/` (e.g., `stagefreight/update-badges`)
- **Bot identity:** StageFreight — every MR it opens, every comment it posts, is brand exposure embedded in developer workflows.
