# StageFreight — Docker Build Configuration

Complete reference for the `docker:` block in `.stagefreight.yml`.

---

## Top-Level Fields

```yaml
docker:
  # Build context directory (relative to project root)
  # Default: "."
  context: "."

  # Dockerfile path (relative to context)
  # Default: auto-detected
  dockerfile: "Dockerfile"

  # Multi-stage build target (--target)
  # Default: none (builds final stage)
  target: ""

  # Target platforms for multi-arch builds
  # Default: current OS/arch
  platforms:
    - linux/amd64
    - linux/arm64

  # Build arguments injected into the Dockerfile
  build_args:
    GO_VERSION: "1.22"
    BUILD_DATE: "{env:BUILD_DATE}"

  registries:
    # ...

  cache:
    # ...

  readme:
    # Covered in Narrator.md (docker.readme)
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `context` | string | `"."` | Build context directory |
| `dockerfile` | string | auto-detected | Dockerfile path relative to context |
| `target` | string | — | Multi-stage target (maps to `--target`) |
| `platforms` | list of string | current arch | Platform list (e.g., `linux/amd64`) |
| `build_args` | map | `{}` | Key-value build arguments |
| `registries` | list | `[]` | Push targets (see below) |
| `cache` | object | — | Build cache settings (see below) |
| `readme` | object | — | README sync to registries (see [Narrator.md](Narrator.md#docker-hub-readme-sync-dockerreadme)) |

---

## Registries

Each entry defines a registry push target with branch/tag gating and retention.

```yaml
  registries:
    - url: "docker.io"
      path: "myorg/myapp"
      credentials: "DOCKERHUB"            # → DOCKERHUB_USER / DOCKERHUB_PASS
      provider: "dockerhub"
      tags:
        - "{version}"
        - "{major}.{minor}"
        - "latest"

      # Push only on certain branches
      branches:
        - "^main$"

      # Push only on certain git tags
      git_tags:
        - "^v\\d+\\.\\d+\\.\\d+$"

      # Tag retention (restic-style)
      retention:
        keep_last: 10
        keep_monthly: 6

    - url: "ghcr.io"
      path: "myorg/myapp"
      credentials: "GHCR"
      provider: "ghcr"
      description: "Short description override for this registry"
      tags:
        - "{version}"
```

### Registry Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `url` | string | _(required)_ | Registry hostname (e.g., `docker.io`, `ghcr.io`) |
| `path` | string | _(required)_ | Image path (e.g., `myorg/myapp`) |
| `tags` | list of string | `["{version}"]` | Tag templates (supports `{version}`, `{major}`, etc.) |
| `credentials` | string | — | Env var prefix for auth (e.g., `"DOCKERHUB"` → `DOCKERHUB_USER`/`DOCKERHUB_PASS`) |
| `provider` | string | auto-detected | Registry vendor (see below) |
| `description` | string | — | Per-registry short description override for README sync |
| `branches` | list of pattern | `[]` (always) | Branch filter (see [Pattern Syntax](#pattern-syntax)) |
| `git_tags` | list of pattern | `[]` (all tags) | Git tag filter |
| `retention` | int or policy | — | Tag cleanup policy (see [Retention Policy](#retention-policy)) |

### Provider Values

| Provider | Registry |
|----------|----------|
| `dockerhub` | Docker Hub |
| `ghcr` | GitHub Container Registry |
| `gitlab` | GitLab Container Registry |
| `quay` | Quay.io |
| `harbor` | Harbor |
| `jfrog` | JFrog Artifactory |
| `gitea` | Gitea Container Registry |
| `generic` | Any OCI registry |

---

## Build Cache

Controls cache invalidation rules for incremental builds.

```yaml
  cache:
    # Auto-detect cache-busting from lockfiles
    # Default: true
    auto_detect: true

    # Explicit cache-busting rules
    watch:
      - paths: ["go.sum"]
        invalidates: ["COPY go.* ./", "RUN go mod download"]

      - paths: ["package-lock.json"]
        invalidates: ["COPY package*.json ./", "RUN npm ci"]
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `auto_detect` | bool | `true` | Auto-detect lockfile-based cache busting |
| `watch` | list of rule | `[]` | Explicit cache-busting rules |

### Watch Rule Fields

| Field | Type | Description |
|-------|------|-------------|
| `paths` | list of glob | Files to watch for changes |
| `invalidates` | list of string | Dockerfile instruction patterns to invalidate when paths change |

---

## Build Strategy

StageFreight selects a build strategy automatically:

| Condition | Strategy | Behavior |
|-----------|----------|----------|
| `--local` flag | **local** | `--load` into daemon, no push |
| Single platform + registries | **load + push** | `--load` then `docker push` each tag |
| Multi-platform + registries | **multi-platform push** | `--push` directly (can't `--load` multi-arch) |
| No registries | **local** | `--load`, default tag `stagefreight:dev` |

---

## Retention Policy

Used by both `docker.registries[].retention` and `release.retention`. Policies
are additive (restic-style) — a tag/release survives if **any** rule wants to
keep it.

Accepts either a plain integer (shorthand for `keep_last`) or a policy map:

```yaml
# Shorthand
retention: 10                    # keep last 10

# Full policy
retention:
  keep_last: 3                   # keep the 3 most recent
  keep_daily: 7                  # keep one per day for 7 days
  keep_weekly: 4                 # keep one per week for 4 weeks
  keep_monthly: 6                # keep one per month for 6 months
  keep_yearly: 2                 # keep one per year for 2 years
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `keep_last` | int | `0` | Keep the N most recent |
| `keep_daily` | int | `0` | Keep one per day for N days |
| `keep_weekly` | int | `0` | Keep one per week for N weeks |
| `keep_monthly` | int | `0` | Keep one per month for N months |
| `keep_yearly` | int | `0` | Keep one per year for N years |

Zero/empty = no cleanup.

---

## Pattern Syntax

Used by `branches`, `git_tags`, and all conditional fields across StageFreight.

```yaml
"^main$"              # regex match (default)
"!^feature/.*"        # negated regex (! prefix)
"main"                # literal match (no regex metacharacters)
"!develop"            # negated literal
```

Empty list = no filter (always matches).

Multiple patterns in a list: evaluated in order, first match wins. Negated
patterns reject, positive patterns accept.

---

## CLI Commands

### `docker build`

```
stagefreight docker build [path] [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--local` | bool | `false` | Build for current platform, load into daemon |
| `--platform` | string slice | from config | Override platforms (comma-separated) |
| `--tag` | string slice | from config | Override/add tags |
| `--target` | string | from config | Override Dockerfile target stage |
| `--skip-lint` | bool | `false` | Skip pre-build lint gate |
| `--dry-run` | bool | `false` | Show the build plan without executing |

### `docker readme`

```
stagefreight docker readme [path] [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | `false` | Show prepared content without pushing |

---

## Pipeline Phases

During `stagefreight docker build`, these phases run in order:

1. **Lint** — pre-build lint gate (skippable with `--skip-lint`)
2. **Detect** — find Dockerfiles, detect language, resolve context
3. **Plan** — resolve platforms, tags, registries, build strategy
4. **Build** — execute `docker buildx` with layer-parsed output
5. **Push** — push tags to remote registries (single-platform load+push)
6. **Badges** — generate configured badge SVGs
7. **README Sync** — compose badges, rewrite links, push to registries
8. **Retention** — prune old tags per retention policies
