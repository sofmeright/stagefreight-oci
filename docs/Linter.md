# StageFreight — Linter Configuration

Complete reference for the `lint:` block in `.stagefreight.yml`.

---

## Top-Level Fields

```yaml
lint:
  # Scan mode
  #   "changed" → only files modified in the current diff
  #   "full"    → scan all files (delta filter disabled)
  level: changed

  # Override cache directory (optional)
  # Default: $XDG_CACHE_HOME/stagefreight/<repo-hash>/lint
  # Env override: STAGEFREIGHT_CACHE_DIR
  cache_dir: ""

  # Glob patterns to exclude from lint scanning
  exclude:
    - "vendor/**"
    - "*.generated.go"

  modules:
    # ...
```

| Field | Type | Default | Notes |
|-------|------|---------|-------|
| `level` | `"changed"` \| `"full"` | `"changed"` | CLI `--level` overrides config |
| `cache_dir` | string | XDG default | Relative to repo root unless absolute |
| `exclude` | list of glob | `[]` | Matched against relative path and basename |

---

## Modules

Each module key under `modules:` accepts:

```yaml
modules:
  <name>:
    enabled: true|false    # default varies per module
    options:               # module-specific config (optional)
      # ...
```

### Content-Only Modules (Cache Forever by Hash)

These modules produce deterministic output from file content alone.
Results are cached indefinitely by content hash — same file always
produces the same findings.

```yaml
    secrets:
      enabled: true

    conflicts:
      enabled: true

    filesize:
      enabled: true

    linecount:
      enabled: false

    tabs:
      enabled: true

    unicode:
      enabled: true

    yaml:
      enabled: true

    lineendings:
      enabled: true
```

#### File Size Module

Detects files that exceed a configurable size threshold.

```yaml
    filesize:
      enabled: true
      options:
        max_bytes: 524288    # bytes; default: 524288 (500 KB)
```

| Option | Type | Default | Notes |
|--------|------|---------|-------|
| `max_bytes` | int (bytes) | `524288` (500 KB) | 0 → use default; negative values rejected |

#### Line Count Module

Detects files that exceed a configurable line count threshold.
**Default disabled** — opt-in only.

```yaml
    linecount:
      enabled: true
      options:
        max_lines: 500       # default: 1000
```

| Option | Type | Default | Notes |
|--------|------|---------|-------|
| `max_lines` | int | `1000` | 0 → use default; negative values rejected |

#### Unicode Module

Detects invisible, confusable, and dangerous Unicode characters —
supply-chain defense against trojan-source attacks, invisible text
obfuscation, and control byte smuggling.

##### Detection Categories

| Category | Config Key | Default | Severity | Allowlist-Bypassable |
|----------|-----------|---------|----------|---------------------|
| BiDi overrides | `detect_bidi` | `true` | critical | No |
| Zero-width chars | `detect_zero_width` | `true` | critical | No |
| ASCII control bytes | `detect_control_ascii` | `true` | warning | Yes (path-scoped) |
| Tag characters | — | always on | critical | No |
| Confusable whitespace | — | always on | warning | No |
| Invalid UTF-8 | — | always on | warning | No |

The three toggleable categories (`detect_bidi`, `detect_zero_width`,
`detect_control_ascii`) default to `true`. All other categories are
always on — there is no toggle or allowlist bypass for them.

##### Path-Scoped ASCII Control Allowlist

The allowlist gates **only** `ASCII control bytes`. BiDi, zero-width,
and all other categories fire regardless of path — even if the file
is on the allowlist.

```yaml
    unicode:
      enabled: true
      options:
        detect_bidi: true
        detect_zero_width: true
        detect_control_ascii: true

        # Paths where specific control bytes are permitted:
        allow_control_ascii_in_paths:
          - "src/output/banner_art.go"

        # Which bytes to allow (0–31, excluding 9/10/13):
        allow_control_ascii: [27]   # ESC only
```

`allow_control_ascii_in_paths` accepts glob patterns (same syntax as
top-level `exclude:`). A control byte is suppressed only when **both**
the byte appears in `allow_control_ascii` **and** the file path matches
at least one pattern.

##### Config Validation

| Constraint | Error |
|------------|-------|
| Value outside 0–31 | rejected (must be ASCII control range) |
| Tab (9), newline (10), CR (13) | rejected — always ignored by the scanner; listing them misleads readers |
| Duplicate values | silently de-duped (map-based) |

##### Example: Allowing ESC in ANSI Art Files

CLI tools that embed prerendered ANSI art (e.g., terminal banners with
color sequences) may need to allow `ESC` (0x1B) in specific files.

```yaml
    unicode:
      options:
        detect_control_ascii: true
        allow_control_ascii_in_paths:
          - "src/output/banner_art.go"
        allow_control_ascii: [27]   # ESC only
```

The exception is intentionally narrow:

- Only ESC (27) is allowed — no other control byte
- Only in the listed path(s) — everywhere else, ESC is flagged
- BiDi and zero-width still fire as critical even in allowed paths

**Preferred alternative**: generate ANSI art at build time using escaped
Go string literals (`\x1b[...` not raw ESC bytes). This eliminates the
need for any allowlist — StageFreight itself uses this approach.

---

### Freshness Module (External-State, TTL-Aware)

The freshness module depends on external registries and CVE feeds.
Its cache entries expire based on the `cache_ttl` setting.

```yaml
    freshness:
      enabled: true

      options:

        # ── Cache TTL ──────────────────────────────────────────
        # Seconds. Controls how long freshness results are cached.
        #
        #   >0 → cache with expiry
        #    0 → cache forever (content-hash only)
        #   <0 → never cache (always re-run)
        #
        # Default: 300 (5 minutes)
        cache_ttl: 300

        # HTTP timeout (seconds) for external lookups
        # Default: 10
        timeout: 10

        # ── Source Ecosystems ──────────────────────────────────
        # Toggle individual dependency ecosystems.
        # Omitted fields default to true.
        sources:
          docker_images: true
          docker_tools: true
          go_modules: true
          rust_crates: true
          npm_packages: true
          alpine_apk: true
          debian_apt: true
          pip_packages: true

        # ── Severity Mapping ───────────────────────────────────
        # Maps version-delta levels to lint severities.
        # Values: 0 = info, 1 = warning, 2 = critical
        severity:
          major: 2              # default: 2 (critical)
          minor: 1              # default: 1 (warning)
          patch: 0              # default: 0 (info)
          major_tolerance: 0    # versions behind before reporting
          minor_tolerance: 0
          patch_tolerance: 1    # default: skip 1 patch behind

        # ── Vulnerability Correlation ──────────────────────────
        # Cross-references dependencies against the OSV database.
        vulnerability:
          enabled: true                 # default: true
          min_severity: "moderate"      # low | moderate | high | critical
          severity_override: true       # CVE-affected deps escalate to critical

        # ── Registry Overrides ─────────────────────────────────
        # Override default public registry URLs per ecosystem.
        # auth_env names an environment variable holding a Bearer token.
        registries:
          docker:
            url: "https://jcr.pcfae.com/v2"
            auth_env: "JCR_TOKEN"
          go:
            url: "https://proxy.golang.org"
          npm:
            url: "https://registry.npmjs.org"
            auth_env: "NPM_TOKEN"
          pypi:
            url: "https://pypi.org/simple"
          crates:
            url: "https://crates.io/api/v1"
          alpine:
            url: ""
          debian:
            url: ""
          ubuntu:
            url: ""
          github:
            url: ""
            auth_env: "GITHUB_TOKEN"

        # ── Ignore ─────────────────────────────────────────────
        # Glob patterns matched against dependency name.
        # Evaluated before package rules.
        ignore:
          - "mock-*"
          - "*.test"

        # ── Package Rules ──────────────────────────────────────
        # Ordered list. First match wins. All match fields are AND'd.
        package_rules:

          - match_packages: ["golang", "alpine"]
            severity:
              major: 2
              minor: 2
              patch: 1

          - match_packages: ["*.test", "mock-*"]
            enabled: false

          - match_ecosystems: ["gomod"]
            match_update_types: ["patch"]
            automerge: true
            group: "go-patch-updates"

          - match_vulnerability: true
            severity:
              major: 2
              minor: 2

        # ── Groups ─────────────────────────────────────────────
        # Named batching groups for future MR creation.
        groups:
          - name: "go-patch-updates"
            commit_message_prefix: "deps(go): "
            automerge: true

          - name: "docker-base-images"
            separate_major: true
```

#### Package Rule Match Fields

| Field | Type | Description |
|-------|------|-------------|
| `match_packages` | list of glob | Dependency name patterns |
| `match_ecosystems` | list of string | `docker-image`, `docker-tool`, `gomod`, etc. |
| `match_update_types` | list of string | `major`, `minor`, `patch` |
| `match_vulnerability` | bool | `true` = only deps with known CVEs |

#### Package Rule Override Fields

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | `false` = skip this dependency entirely |
| `severity` | object | Override `severity` block for matched deps |
| `group` | string | Assign to a named group |
| `automerge` | bool | Auto-merge MR if CI passes (future) |

---

## Cache TTL Contract

The `CacheTTLModule` interface controls time-based cache expiry for
modules that depend on external state.

| `CacheTTL()` Return | Engine Behavior | Expiry Logic |
|----------------------|-----------------|--------------|
| `> 0` | Cache with TTL | Expires when `now - CachedAt > TTL` |
| `== 0` | Cache forever | No expiry check |
| `< 0` | Never cache | Skip Get + Put |
| Not implemented | Cache forever | No expiry check |

Modules that do not implement `CacheTTLModule` are cached forever by
content hash. This is correct for deterministic modules (secrets,
conflicts, filesize) where the same file content always produces the
same findings.

External-state modules (freshness) implement `CacheTTLModule` to declare
their TTL. Cache entries store a `CachedAt` timestamp. Old entries
written before TTL support (missing timestamp) are treated as expired
for TTL-aware modules, ensuring safe migration.

---

## CLI Flags

```
stagefreight lint [paths...] [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--level` | string | from config, then `"changed"` | `changed` or `full` |
| `--module` | string slice | all enabled | Run only these modules |
| `--no-module` | string slice | none | Skip these modules |
| `--no-cache` | bool | `false` | Clear cache and rescan |
| `--all` | bool | `false` | Shorthand for `--level full` |

**Precedence**: `--level` flag > `lint.level` config > `"changed"` default.
