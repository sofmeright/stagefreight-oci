# StageFreight — Configuration Reference

## `.stagefreight.yml`

Three top-level keys. `extends` flows org policy down. `adherence` is how
strict. `repository` is what to scan.

---

### `extends`

| Type | Default |
|------|---------|
| list of string (URL or path) | `[]` |

Inherit configuration from external presets. Resolved in order — later
entries override earlier ones. Local config overrides all inherited values.

Merge strategy:
- Local rules prepend to inherited rules (first-match-wins semantics)
- Local scalars replace inherited values
- Local maps merge with inherited maps

URLs are fetched at scan time. Paths are relative to the repository root.

---

### `adherence`

Global policy layer. Controls how strictly StageFreight enforces findings
across all modules.

---

#### `adherence.vulnerabilities`

| Field | Type | Default |
|-------|------|---------|
| `enabled` | bool | `true` |
| `threshold` | string | `"moderate"` |
| `zero_tolerance` | bool | `true` |

`threshold` — CVE severity floor. Values: `low`, `moderate`, `high`,
`critical`.

`zero_tolerance` — Any finding associated with a CVE at or above threshold
is escalated to critical and reported regardless of module-specific tolerance.

---

#### `adherence.freshness`

##### `adherence.freshness.<major|minor|patch>`

| Field | Type | Major Default | Minor Default | Patch Default |
|-------|------|---------------|---------------|---------------|
| `severity` | int | `2` | `1` | `0` |
| `tolerance` | int | `0` | `0` | `1` |

Severity values: `0` = info, `1` = warning, `2` = critical.
Tolerance: number of versions behind before a finding is emitted.

##### `adherence.freshness.versioning`

| Field | Type | Default |
|-------|------|---------|
| `precision` | string | `"expand"` |

`precision` — Global default for version segment matching.

| Value | Behavior |
|-------|----------|
| `match` | Only suggest same segment count (`7` -> `8`, not `7.4.2`) |
| `expand` | Suggest highest at any precision |
| `ignore` | Skip freshness for this dependency |

---

#### `adherence.lint`

##### `adherence.lint.tabs`

| Field | Type | Default |
|-------|------|---------|
| `severity` | int | `0` |

##### `adherence.lint.line_endings`

| Field | Type | Default |
|-------|------|---------|
| `severity` | int | `0` |

##### `adherence.lint.unicode`

| Field | Type | Default |
|-------|------|---------|
| `severity` | int | `2` |

##### `adherence.lint.conflicts`

| Field | Type | Default |
|-------|------|---------|
| `severity` | int | `2` |

##### `adherence.lint.large_files`

| Field | Type | Default |
|-------|------|---------|
| `severity` | int | `1` |
| `threshold` | string | `"5MB"` |

`threshold` — File size above which a finding is emitted. Supports `KB`,
`MB`, `GB` suffixes.

---

### `repository`

Detection configuration — what to scan, where to look, how to report.

---

#### `repository.sources`

Toggle dependency ecosystems. Omitted fields default to `true`.

| Field | Type | Default |
|-------|------|---------|
| `docker_images` | bool | `true` |
| `docker_tools` | bool | `true` |
| `go_modules` | bool | `true` |
| `rust_crates` | bool | `true` |
| `npm_packages` | bool | `true` |
| `alpine_apk` | bool | `true` |
| `debian_apt` | bool | `true` |
| `pip_packages` | bool | `true` |

---

#### `repository.registries`

Registry configuration keyed by host. StageFreight resolves the registry
from the image reference and looks up config by host match. Public defaults
are used for hosts not listed.

##### RegistryEndpoint

| Field | Type | Default |
|-------|------|---------|
| `auth_env` | string | _(none)_ |
| `fetch_annotations` | bool | `false` |

`auth_env` — Name of environment variable holding a Bearer token.

`fetch_annotations` — Fetch OCI manifest annotations
(`org.opencontainers.image.version`, `org.opencontainers.image.source`)
from this registry.

##### Common hosts

| Key | Matches |
|-----|---------|
| `"registry.hub.docker.com"` | Docker Hub (including shorthand `library/` images) |
| `"ghcr.io"` | GitHub Container Registry |
| `"quay.io"` | Quay.io |
| `"proxy.golang.org"` | Go module proxy |
| `"registry.npmjs.org"` | npm registry |
| `"pypi.org"` | Python Package Index |
| `"crates.io"` | Rust crate registry |
| `"dl-cdn.alpinelinux.org"` | Alpine APK repositories |
| `"deb.debian.org"` | Debian APT repositories |
| `"archive.ubuntu.com"` | Ubuntu APT repositories |
| `"api.github.com"` | GitHub Releases API (tool version checks) |

Any hostname can be added for private registries.

---

#### `repository.ignore`

| Type | Default |
|------|---------|
| list of string (glob) | `[]` |

Dependency names matching any pattern are skipped entirely. No findings
of any kind. Evaluated before package rules.

---

#### `repository.package_rules[]`

Ordered list. First match wins. All specified match fields are AND'd.

##### Match fields

| Field | Type |
|-------|------|
| `match_packages` | list of string (glob) |
| `match_ecosystems` | list of string |
| `match_update_types` | list of string (`major`, `minor`, `patch`) |
| `match_vulnerability` | bool |
| `match_files` | list of string (glob) |

`match_files` — Glob patterns matched against the file path containing
the dependency. Enables per-Dockerfile or per-directory policy scoping.

##### Override fields

| Field | Type |
|-------|------|
| `enabled` | bool |
| `major` | object (`{ severity, tolerance }`) |
| `minor` | object (`{ severity, tolerance }`) |
| `patch` | object (`{ severity, tolerance }`) |
| `vulnerabilities` | object (`{ enabled, threshold, zero_tolerance }`) |
| `versioning` | object (see below) |
| `group` | string |
| `automerge` | bool |

Override fields are partial — only specified fields override the global
`adherence` values. Unspecified fields inherit.

##### `package_rules[].versioning`

Per-image tag interpretation. Overrides auto-detection.

| Field | Type |
|-------|------|
| `extract` | string (regex with `(?P<version>...)` capture) |
| `compare` | string (`semver`, `chronological`, `alphabetical`, `numerical`) |
| `exclude` | list of string (regex) |
| `precision` | string (`match`, `expand`, `ignore`) |
| `allowed_versions` | string (semver constraint) |

`extract` — Named capture group pulls the comparable value from the raw tag.

`compare` — How extracted values are sorted to find the latest.

`exclude` — Tags matching any pattern are dropped from the candidate set.

`precision` — Overrides `adherence.freshness.versioning.precision`.

`allowed_versions` — Semver constraint applied to candidates. Tags outside
this range are excluded. Example: `"<2.0.0"`, `">=1.0.0 <3.0.0"`.

---

#### `repository.groups[]`

Named groups for MR batching.

| Field | Type | Default |
|-------|------|---------|
| `name` | string | _(required)_ |
| `commit_message_prefix` | string | `""` |
| `automerge` | bool | `false` |
| `separate_major` | bool | `false` |

---

#### `repository.advisory`

Informational findings and noise tuning.

| Field | Type | Default |
|-------|------|---------|
| `stable_upgrade` | bool | `true` |
| `pin_suggestion` | bool | `true` |
| `digest_pin` | bool | `false` |
| `changelog_links` | bool | `false` |
| `deduplicate_digests` | bool | `false` |
| `suppress` | list of string (glob) | `[]` |

`stable_upgrade` — Suggest stable releases for sha-pinned and pre-release tags.

`pin_suggestion` — Suggest versioned tags for `latest`, codenames, etc.

`digest_pin` — Report on `image@sha256:` references. When `true`, advises
adding a comment with the source tag for auditability. When `false`,
digest-pinned images are silently accepted as intentional.

`changelog_links` — Enrich findings with release notes links from
`org.opencontainers.image.source` annotations. Requires
`fetch_annotations: true` on the relevant registry.

`deduplicate_digests` — Suppress findings where current and suggested tags
resolve to the same manifest digest. Requires a HEAD request per candidate.

`suppress` — Silence advisory findings for matched image names.

##### `repository.advisory.digest_drift`

| Field | Type | Default |
|-------|------|---------|
| `non_versioned` | bool | `true` |
| `versioned` | bool | `false` |

`non_versioned` — Track digest changes for non-versioned tags.

`versioned` — Detect same-tag rebuilds on versioned tags.

---

#### `repository.timeout`

| Type | Default |
|------|---------|
| int | `10` |

HTTP request timeout in seconds for all registry and API calls.
