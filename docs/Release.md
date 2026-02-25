# StageFreight — Release Configuration

Complete reference for the `release:` block in `.stagefreight.yml`.

---

## Top-Level Fields

```yaml
release:
  # Badge configuration
  badge:
    enabled: true                             # commit badge SVG to repo (default: true)
    path: ".stagefreight/badges/release.svg"  # badge file path (default)
    branch: "main"                            # branch to commit to (default: main)

  # Rolling tag templates (resolved against git version info)
  tags:
    - "{version}"
    - "{major}.{minor}"
    - "latest"

  # Git tag filter for auto-tagging
  git_tags:
    - "^v\\d+\\.\\d+\\.\\d+$"        # stable semver only
    # - "!^v.*-rc"                     # exclude release candidates

  # Sync targets (cross-forge release sync)
  sync:
    # ...

  # Release retention (restic-style)
  retention:
    keep_last: 10
    keep_monthly: 6
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `badge` | object | see below | Release badge SVG configuration |
| `tags` | list of template | `[]` | Rolling tag templates (see [Template Variables](Narrator.md#template-variables)) |
| `git_tags` | list of pattern | `[]` (all) | Git tag filter for auto-tagging (see [Pattern Syntax](Docker.md#pattern-syntax)) |
| `sync` | list of target | `[]` | Cross-forge sync targets |
| `retention` | int or policy | — | Release cleanup (see [Retention Policy](Docker.md#retention-policy)) |

---

## Badge

Controls the release status badge SVG committed to the repository after each
release. The badge is generated with the detected version and committed via
the forge API (no local clone needed).

```yaml
  badge:
    enabled: true                             # default: true
    path: ".stagefreight/badges/release.svg"  # default
    branch: "main"                            # default: main
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Commit badge SVG to repo |
| `path` | string | `.stagefreight/badges/release.svg` | Badge file path in repo |
| `branch` | string | `main` | Branch to commit badge to |

---

## Sync Targets

Each target defines a forge to mirror releases, badges, and scan artifacts to.

```yaml
  sync:
    - name: "GitHub Mirror"
      provider: "github"
      url: "https://github.com"
      credentials: "GITHUB_SYNC"      # → GITHUB_SYNC_TOKEN env var
      project_id: "myorg/myapp"

      # Branch/tag filters
      branches:
        - "^main$"
      tags:
        - "^v\\d+\\.\\d+\\.\\d+$"

      # What to sync
      sync_release: true               # sync release notes and tags (default: true)
      sync_assets: true                # sync scan artifacts (SARIF, SBOM) (default: true)
      sync_badge: false                # commit badge SVG to this target (default: false)
```

### Sync Target Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | _(required)_ | Human-readable label |
| `provider` | string | _(required)_ | Forge type: `gitlab`, `github`, `gitea` |
| `url` | string | _(required)_ | Forge base URL |
| `credentials` | string | — | Env var prefix (e.g., `"GITHUB_SYNC"` → `GITHUB_SYNC_TOKEN`) |
| `project_id` | string | from env | Project identifier (`owner/repo` or numeric ID) |
| `branches` | list of pattern | `[]` (always) | Branch filter |
| `tags` | list of pattern | `[]` (all) | Tag filter |
| `sync_release` | bool | `true` | Sync release notes and tags |
| `sync_assets` | bool | `true` | Upload scan artifacts to synced release |
| `sync_badge` | bool | `false` | Commit badge SVG to this target |

---

## CLI Commands

### `release create`

Create a release on the detected forge with auto-generated or provided release
notes. Uploads assets, adds registry links, creates rolling tags, syncs to
targets, and applies retention.

```
stagefreight release create [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--tag` | string | auto-detected | Release tag (default: `v{version}` from git) |
| `--name` | string | same as tag | Release name |
| `--notes` | string | auto-generated | Path to release notes markdown file |
| `--security-summary` | string | — | Path to security output directory (reads `summary.md`) |
| `--draft` | bool | `false` | Create as draft release |
| `--prerelease` | bool | `false` | Mark as prerelease |
| `--asset` | string slice | `[]` | Files to attach to release (repeatable) |
| `--registry-links` | bool | `true` | Add registry image links to release |
| `--catalog-links` | bool | `true` | Add GitLab Catalog link to release |
| `--skip-sync` | bool | `false` | Skip syncing to other forges |

### `release badge`

Generate and commit a release status badge SVG via the forge API.

```
stagefreight release badge [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--version` | string | auto-detected | Version to display |
| `--status` | string | `passed` | Badge status: `passed`, `warning`, `critical` |
| `--path` | string | from config | Repo path for badge file |
| `--branch` | string | from config | Branch to commit badge to |
| `--local` | bool | `false` | Write to local filesystem instead of forge API |

### `release notes`

Generate markdown release notes from conventional commits between two refs.

```
stagefreight release notes [path] [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--from` | string | previous tag | Start ref |
| `--to` | string | `HEAD` | End ref |
| `--security-summary` | string | — | Path to security summary markdown to embed |
| `-o`, `--output` | string | stdout | Write notes to file |

### `release prune`

Delete old releases using the configured retention policy.

```
stagefreight release prune [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | `false` | Show what would be deleted without deleting |

---

## Release Create Flow

1. Detect version from git
2. Generate or load release notes (conventional commits)
3. Create release on detected forge (GitLab, GitHub, Gitea)
4. Upload asset files (SARIF, SBOM, etc.)
5. Add registry image links (one per configured registry)
6. Add GitLab Catalog link (if `component.catalog: true`)
7. Create rolling tags from `release.tags` templates
8. Sync release to configured `release.sync` targets
9. Apply `release.retention` policy (auto-prune old releases)
