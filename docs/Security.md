# StageFreight — Security Scanning Configuration

Complete reference for the `security:` block in `.stagefreight.yml`.

---

## Top-Level Fields

```yaml
security:
  # Run vulnerability scanning (default: true)
  enabled: true

  # Generate SBOM artifacts via Syft (default: true)
  sbom: true

  # Fail the pipeline if critical vulnerabilities are found
  # Default: false
  fail_on_critical: false

  # Output directory for scan artifacts (JSON, SARIF, SBOM, summary)
  # Default: .stagefreight/security
  output_dir: ".stagefreight/security"

  # Default detail level for security info in release notes
  # Values: "none", "counts", "detailed", "full"
  # Default: "counts"
  release_detail: "counts"

  # Conditional detail level overrides (first match wins)
  release_detail_rules:
    - tag: "^v\\d+\\.\\d+\\.\\d+$"       # stable releases get full detail
      detail: "full"

    - branch: "!^main$"                    # non-main branches get counts only
      detail: "counts"

    - tag: "^v.*-rc"                       # release candidates get detailed
      detail: "detailed"
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Run vulnerability scanning |
| `sbom` | bool | `true` | Generate SBOM artifacts (Syft) |
| `fail_on_critical` | bool | `false` | Exit non-zero if critical vulnerabilities found |
| `output_dir` | string | `.stagefreight/security` | Directory for scan artifacts |
| `release_detail` | string | `"counts"` | Default detail level for release notes |
| `release_detail_rules` | list of rule | `[]` | Conditional detail level overrides |

---

## Detail Levels

Controls how much security information is embedded in release notes.

| Level | Description |
|-------|-------------|
| `none` | No security info in release notes |
| `counts` | Vulnerability count summary (e.g., "0 critical, 2 high") |
| `detailed` | Count summary with affected package list |
| `full` | Full vulnerability table with CVE IDs, severity, and descriptions |

---

## Detail Rules

Conditional overrides evaluated top-down (first match wins). Uses the
standard [Condition](#condition) primitive for tag/branch matching.

```yaml
  release_detail_rules:
    - tag: "^v\\d+\\.\\d+\\.\\d+$"    # stable releases
      detail: "full"

    - branch: "^main$"                 # main branch
      detail: "detailed"

    - detail: "counts"                 # catch-all (no conditions)
```

### Detail Rule Fields

| Field | Type | Description |
|-------|------|-------------|
| `tag` | pattern | Git tag filter (only evaluated when a tag is present) |
| `branch` | pattern | Branch filter |
| `detail` | string | Detail level: `none`, `counts`, `detailed`, `full` |

**Precedence**: CLI `--security-detail` flag > first matching rule > `release_detail` default.

---

## Condition

The universal conditional primitive used across StageFreight. Every feature
with tag/branch-sensitive behavior uses this structure.

```yaml
tag: "^v\\d+\\.\\d+\\.\\d+$"     # regex match (default)
branch: "!^feature/.*"            # negated regex (! prefix)
```

| Field | Type | Description |
|-------|------|-------------|
| `tag` | pattern | Matched against `CI_COMMIT_TAG` / git tag. Only evaluated when a tag is present. |
| `branch` | pattern | Matched against `CI_COMMIT_BRANCH` / git branch. |

**Matching logic**:
- Multiple fields set: AND — all present fields must match.
- No fields set: catch-all (always matches).
- Rules are always evaluated top-down, first match wins.

---

## Scan Artifacts

After a scan, the output directory contains:

| File | Format | Description |
|------|--------|-------------|
| `results.json` | Trivy JSON | Raw vulnerability scan results |
| `results.sarif` | SARIF | For GitLab/GitHub security dashboard integration |
| `sbom.json` | CycloneDX | Software Bill of Materials (when `sbom: true`) |
| `summary.md` | Markdown | Human-readable summary at configured detail level |

The `summary.md` is consumed by `release create --security-summary` to embed
security information in release notes.

---

## CLI Commands

### `security scan`

```
stagefreight security scan [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--image` | string | _(required)_ | Image reference or tarball to scan |
| `-o`, `--output` | string | from config | Output directory for artifacts |
| `--sbom` | bool | `true` | Generate SBOM artifacts |
| `--fail-on-critical` | bool | `false` | Exit non-zero if critical vulnerabilities found |
| `--skip` | bool | `false` | Skip scan (for pipeline control) |
| `--security-detail` | string | from config | Override detail level: `none`, `counts`, `detailed`, `full` |
