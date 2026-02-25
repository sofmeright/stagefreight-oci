# Versioning — Tag Classification & Freshness Road Map

> Tracking all work related to container image tag parsing, version comparison,
> family grouping, and upgrade advisory logic in the freshness module.

---

## Competitive Landscape

StageFreight competes with (or replaces the need for) these tools in the
container version tracking space:

| Tool | Approach | Where We Win | Where They Win |
|------|----------|--------------|----------------|
| **Renovate** | Config-driven, per-package versioning schemes | Auto-detection of families without user config; compound suffix handling; pre-release awareness | Massive ecosystem coverage (npm, pip, cargo, etc.); mature MR/PR workflow; `extractVersion` regex escape hatch |
| **Dependabot** | Opinionated, limited config | Any non-trivial Docker tag; family grouping; sha-pinned advisory | Zero-config GitHub integration; release notes linking via OCI `image.source` |
| **Flux Image Automation** | Per-image ImagePolicy CRDs with regex + policy | No per-image CRD boilerplate needed; works outside k8s | Clean two-phase design (`filterTags.extract` + policy); native k8s integration |
| **Watchtower** | Digest-only tracking on current tag | Version-aware cross-tag upgrades; advisory for sha-pinned images | Simpler mental model for "just keep my tag fresh" |
| **WUD (What's Up Docker)** | Registry polling + tag regex filters | Automatic family grouping; pre-release ranking | Nice dashboard UI; broad notification support |
| **Diun** | Regex include/exclude on tags | No regex config needed for common patterns | Lightweight; notification-focused |

### Renovate's Known Weaknesses (Our Advantages)

These are documented bugs/limitations in Renovate that our `normalizeFamily()`
pipeline already handles or that we should ensure we continue to handle:

1. **Compound suffixes break exact matching**
   `docker:24.0.7-dind-alpine3.18` — Renovate requires the entire
   `-dind-alpine3.18` to match exactly. When Alpine bumps to 3.19, Renovate
   finds zero candidates. Our `normalizeFamily` strips the Alpine version,
   producing family `dind-alpine`, so `24.0.7-dind-alpine3.19` groups correctly.
   ([Discussion #26820](https://github.com/renovatebot/renovate/discussions/26820))

2. **Pre-release tags misclassified as compatibility suffixes**
   `1.2.3-beta.1` — Renovate's Docker versioning treats everything after the
   hyphen as a compatibility suffix, not a pre-release indicator. It will never
   suggest upgrading from `beta.1` to the stable `1.2.3`. Our `detectPreRelease`
   and `PreRank` ordering handle this correctly.
   ([Discussion #34832](https://github.com/renovatebot/renovate/discussions/34832))

3. **LinuxServer `lsNNN` rebuild suffixes**
   `kasm:1.18.0-ls117` — Renovate's exact suffix match means `ls117` != `ls118`,
   so no updates are found. Our `normalizeFamily` strips the rebuild counter,
   grouping all `ls*` tags together.

4. **User config required for non-semver**
   MinIO `RELEASE.YYYY-MM-DDTHH-MM-SSZ`, calver `2026.1.30-hash` — Renovate
   requires per-package `packageRules` with custom `versioning: "regex:..."`.
   We detect these automatically in `decomposeTag` stages 1 and 3.

---

## Configuration API

The freshness module's configuration is split across two top-level keys
in `.stagefreight.yml`:

- **`adherence`** — policy layer (how strict am I about what I find)
- **`repository`** — detection layer (what to scan, where to look, how to report)

See [Configuration Reference](ConfigurationReference.md) for the full API
specification. The versioning-relevant surfaces are:

| Path | Purpose |
|------|---------|
| `adherence.freshness.<major\|minor\|patch>` | Severity and tolerance per version-delta axis |
| `adherence.freshness.versioning.precision` | Global default for version segment matching |
| `adherence.vulnerabilities` | Cross-cutting CVE policy (threshold, zero-tolerance) |
| `repository.registries.<host>.fetch_annotations` | Enable OCI annotation fetching per registry |
| `repository.package_rules[].versioning` | Per-image tag extraction, comparison, exclusion, version constraints |
| `repository.advisory` | Advisory toggles (stable upgrade, pin suggestion, digest drift, changelog links) |

---

## Completed Work

### Tag Classification Overhaul (2026-02-24)

Replaced the naive "split on first hyphen, exact suffix match" with a
multi-stage classification pipeline.

**Files modified:** `semver.go`, `images.go`, `dependency.go`, `module.go`

#### Enhanced `decomposedTag` Struct
- `Family` — normalized grouping key (strips hashes, rebuild numbers, version-within-suffix)
- `PreRank` — pre-release ranking: 0=stable, 1=rc, 2=beta, 3=alpha, 4=dev
- `PreNum` — pre-release counter (beta17 -> 17)
- `Suffix` preserved as-is for downstream consumers (`detectAlpineVersion`, `detectDebianDistro`)

#### Classification Pipeline in `decomposeTag()`
1. **MinIO RELEASE detection** — `RELEASE.2025-09-07T16-13-09Z` encoded as semver `20250907.161309.0`
2. **sha- prefix detection** — `sha-37e807f-alpine` -> Version=nil, Family="sha"
3. **Standard decomposition** — split on first hyphen, semver parse, progressive fallback for 4+ dot versions (Plex `1.40.2.8395`)

#### `normalizeFamily()` — Stable Grouping Keys
Strips per-release metadata from raw suffix to produce stable family keys:
- Hex hashes (7-40 chars): `ad42b553b` -> stripped
- Pure numeric segments: `8395` -> stripped
- Trailing digit counters: `beta17` -> `beta`, `ls117` -> `ls`
- Embedded versions: `alpine3.22` -> `alpine`

#### `filterTagsByFamily()` Replaces `filterTagsBySuffix()`
Groups tags by normalized `Family` key instead of exact `Suffix` match.

#### Pre-Release-Aware `latestInFamily()` + `tagNewer()`
- Stable releases beat pre-releases at the same version
- Higher pre-release rank wins (rc > beta > alpha > dev)
- Higher pre-release number wins within same rank (beta17 > beta13)

#### Cross-Pattern Stable Release Advisory (`suggestStableUpgrade`)
- sha-pinned images: "stable releases available (e.g. 25.3.0-alpine)"
- Pre-release tags: "stable release X.Y.Z available (currently on pre-release)"
- Non-versioned tags: "consider pinning to a versioned tag (e.g. X.Y.Z)"

### Configuration API Design (2026-02-24)

Designed the full configuration API through iterative refinement. Key
design decisions:

- **Two top-level keys** (`adherence` / `repository`) separate policy from detection
- **`adherence` is cross-cutting** — vulnerabilities and lint sit as peers to freshness, not nested inside it
- **`package_rules[]` override fields are flat** — no intermediate `adherence` wrapper inside rules; match fields (`match_*`) and override fields (`major`, `versioning`, etc.) are visually distinct by naming convention
- **Registries keyed by host** — not by ecosystem; supports multiple private registries with different auth
- **`extends`** for org-wide preset inheritance — local rules prepend (first-match-wins)
- **`allowed_versions` only per-package** — global version constraints have no sensible use case
- **Max nesting depth of 4** everywhere — uniform ceiling, no 5+ level paths

---

## Known Bugs to Fix

### BUG: Bitnami `rNN` Rebuild Suffix Leaves Stray `r` Prefix

**Severity:** Medium — affects all Bitnami images

Bitnami uses `6.4.3-debian-12-r10` where `rNN` is a rebuild counter (like
LinuxServer's `lsNNN`). Currently `normalizeFamily("debian-12-r10")` produces
`debian-r` instead of the expected `debian`.

The problem: `stripTrailingDigits("r10")` produces `"r"`, which passes through
`stripEmbeddedVersion` unchanged (no digit after alpha prefix since the digits
were already stripped).

**Fix:** Add `rNN` to the known rebuild pattern list alongside `lsNNN`. Either:
- Detect single-char prefix + digits as a rebuild pattern and strip entirely
- Add explicit known prefixes: `r`, `ls`, `rev`, `build`

### BUG: `isHexHash` Only Checks Lowercase

**Severity:** Low — most images use lowercase hashes, but some don't

Our `isHexHash()` only matches `[a-f0-9]`. Tags with uppercase hex chars
(`A-F`) won't be recognized as hashes. Should lowercase the input before
checking, or extend the char range.

### BUG: Release Depth / Precision Mismatch

**Severity:** Medium — affects major-only pins like `redis:7`, `postgres:15`

When someone pins `redis:7` (1 version segment), they want updates within
major 7 but NOT suggestions to jump to `7.4.2` (3 segments) which implies a
different pinning strategy. Currently `redis:7` with Family="" matches
`7.4.2` with Family="" — different precision levels grouped together.

Renovate handles this via "release array length matching": tags must have the
same number of version segments to be considered compatible. Controlled by
`adherence.freshness.versioning.precision` (default: `expand`) and
overridable per-package via `package_rules[].versioning.precision`.

---

## Planned Improvements

### P1: Fix Known Bugs Above

Address the three bugs listed above. Small, targeted fixes in `semver.go`.

### P2: Implement Configuration API

**Status:** API designed (2026-02-24), implementation pending

Implement the new config shape to replace the current `FreshnessConfig` struct.
Key changes:

- Rename `SeverityConfig` fields to match `adherence.freshness.<axis>` shape
- Rename `vulnerability.min_severity` → `threshold`, `severity_override` → `zero_tolerance`
- Restructure `RegistryConfig` from ecosystem-keyed to host-keyed
- Add `match_files` to `PackageRule` match fields
- Add `versioning` struct to `PackageRule` (extract, compare, exclude, precision, allowed_versions)
- Add `Advisory` config (stable_upgrade, pin_suggestion, digest_pin, changelog_links, deduplicate_digests, suppress, digest_drift)
- Add `extends` config loading with merge strategy

### P3: Custom Release Pattern Templates (User Escape Hatch)

**Status:** API designed as `package_rules[].versioning`

For images with truly unique tagging schemes that the classification pipeline
can't auto-detect, users define custom extraction and comparison in package rules:

```yaml
repository:
  package_rules:
    - match_packages: ["minio/minio"]
      versioning:
        extract: '^RELEASE\.(?P<version>\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2})Z$'
        compare: chronological
    - match_packages: ["internal.registry.io/myapp"]
      versioning:
        extract: '^build-(?P<version>\d+)$'
        compare: numerical
        exclude: ['^nightly-']
        allowed_versions: "<2.0.0"
```

The auto-detection pipeline always runs first; custom patterns are a fallback
for the long tail of weird tagging schemes.

### P4: Auditable Failure Messages (Pillar 3)

**Status:** Gap identified

Current error handling is silent — registry timeouts, unparseable tags, and
zero-candidate families produce no user-visible output. Add structured
warnings for:

- Unparseable tags (with the raw tag string and why it failed)
- Registry failures (timeout, auth error, rate limit)
- Zero-candidate families (tag parsed but no comparable tags found)
- Custom regex with no matches
- Digest lock staleness

These are the findings that buy user trust even when the tool fails.

### P5: OCI Manifest Annotation Fetching

**Status:** API designed as `repository.registries.<host>.fetch_annotations`

The OCI spec defines annotations on image manifests that provide ground truth
metadata we can use when tag parsing is ambiguous:

| Annotation | Use Case |
|------------|----------|
| `org.opencontainers.image.version` | Authoritative version string — disambiguates compound tags |
| `org.opencontainers.image.source` | Source repo URL — enables changelog/release notes linking |
| `org.opencontainers.image.created` | Build timestamp — resolves ordering when versions are equal |
| `org.opencontainers.image.base.name` | Base image reference — chain-of-custody for multi-stage builds |

**Implementation approach:**
- v2 manifest HEAD request per image (cheap — no full image pull, just headers + JSON)
- Cache annotations alongside tag list (same TTL as registry polling)
- Use `image.version` as a fallback when `decomposeTag()` returns `Version=nil`
- Use `image.source` to generate changelog links (controlled by `repository.advisory.changelog_links`)

**Differentiation:** Dependabot uses `image.source` for release notes but
nobody uses `image.version` for version detection. This would let us correctly
version images that have completely opaque tags but well-labeled manifests.

### P6: Changelog & Release Notes Linking

**Status:** API designed as `repository.advisory.changelog_links`

Depends on P5. When an upgrade is available, include a link to the changelog
or release notes diff:
- Extract source repo from `org.opencontainers.image.source`
- For GitHub repos: use GitHub Releases API to find release matching the tag
- For GitLab repos: use Releases API similarly
- Include the link in the advisory message or as structured metadata on the Finding

### P7: Multi-Registry Tag Aggregation

**Status:** Idea

Some images publish to multiple registries (Docker Hub, GHCR, Quay) with
different tag sets. When checking freshness, aggregate tags from all known
mirrors to get the most complete picture. This matters for images that
publish pre-releases only to GHCR but stable releases to Docker Hub.

### P8: Digest-Based Rebuild Detection for Versioned Tags

**Status:** API designed as `repository.advisory.digest_drift.versioned`

Currently `lockfile.go` only tracks digests for non-versioned tags. But
versioned tags can also be rebuilt (security patches to `3.22.1` without a
version bump). Extend digest tracking to versioned tags as an optional
SeverityInfo advisory: "redis:7.4.2 has been rebuilt since last check."

This is what Watchtower and WUD do, but they can't do version comparison.
We'd be the first tool to do both.

### P9: Digest Deduplication

**Status:** API designed as `repository.advisory.deduplicate_digests`

Suppress findings where current and suggested tags resolve to the same
manifest digest. Prevents noisy false positives when alias tags (e.g.
`redis:7.4` and `redis:7.4.2`) point to the same image. Requires a
HEAD request per candidate.

### P10: Preset / Extends System

**Status:** API designed as top-level `extends`

For org-wide policy distribution across multiple repos. Presets are
fetched by URL or path at scan time. Merge strategy:
- Local rules prepend to inherited rules (first-match-wins)
- Local scalars replace inherited values
- Local maps merge with inherited maps

Enables the orchestrator (`stagefreight serve`) to push policy down to
agents via hosted preset URLs.

---

## Tag Pattern Coverage Matrix

Tracking which real-world tag patterns we handle correctly. Drawn from the
dungeon cluster (323+ images) and popular Docker Hub images.

| Pattern | Example | Status | Notes |
|---------|---------|--------|-------|
| Standard semver | `alpine:3.22.1` | DONE | Basic case |
| Semver + distro suffix | `golang:1.25-alpine` | DONE | Family="alpine" |
| Semver + distro version | `golang:1.25-alpine3.22` | DONE | Family="alpine", Suffix preserved |
| Debian codename suffix | `golang:1.25-bookworm` | DONE | Family="bookworm" |
| Calver + commit hash | `searxng:2026.1.30-ad42b553b` | DONE | Hash stripped, Family="" |
| MinIO RELEASE | `minio:RELEASE.2025-09-07T16-13-09Z` | DONE | Stage 1 detection |
| Pre-release + number | `pelican:v1.0.0-beta17` | DONE | PreRank=2, PreNum=17 |
| sha- prefix | `actual-server:sha-37e807f-alpine` | DONE | Family="sha", Version=nil |
| Plex compound 4-dot | `pms-docker:1.40.2.8395-c67dce28e` | DONE | Progressive parse |
| LinuxServer rebuild | `kasm:1.18.0-ls117` | DONE | Family="ls" |
| Major-only pin | `redis:7` | BUG | Matches wrong precision |
| Bitnami rebuild | `redis:6.4.3-debian-12-r10` | BUG | Family="debian-r" not "debian" |
| Uppercase hex hash | `app:1.2.3-ABCDEF1` | BUG | isHexHash misses uppercase |
| Multi-version compound | `sd-webui:v2-cuda-12.1.1-base-22.04-v1.10.1` | UNTESTED | Needs investigation |
| Ubuntu codename | `ubuntu:noble` | DONE | Non-versioned, digest tracking |
| `latest` tag | `nginx:latest` | DONE | Digest tracking + advisory |
| Bare version no suffix | `postgres:15.3` | DONE | Family="" |
| Bitnami immutable | `redis:7.4.3-debian-12-r10` | BUG | Same as Bitnami rebuild |
| Node.js compound | `node:20.11.0-alpine3.18` | DONE | Family="alpine" |
| Distro + slim | `python:3.12-slim-bookworm` | DONE | Family="slim-bookworm" |
| RC dot notation | `app:1.0.0-rc.3` | DONE | PreRank=1, PreNum=3 |
| Digest pin | `golang@sha256:abc123...` | DONE | Advisory controlled by `repository.advisory.digest_pin` |

---

## References

- [Renovate Docker Versioning](https://docs.renovatebot.com/docker/)
- [Renovate Versioning Schemes](https://docs.renovatebot.com/modules/versioning/)
- [Renovate Regex Versioning](https://docs.renovatebot.com/modules/versioning/regex/)
- [Renovate Compound Suffix Bug #26820](https://github.com/renovatebot/renovate/discussions/26820)
- [Renovate Pre-Release Bug #34832](https://github.com/renovatebot/renovate/discussions/34832)
- [Flux ImagePolicy Docs](https://fluxcd.io/flux/components/image/imagepolicies/)
- [Dependabot Docker Limitations #3929](https://github.com/dependabot/dependabot-core/issues/3929)
- [OCI Image Annotations Spec](https://specs.opencontainers.org/image-spec/annotations/)
- [Bitnami Rolling Tags](https://techdocs.broadcom.com/us/en/vmware-tanzu/bitnami-secure-images/bitnami-secure-images/services/bsi-doc/apps-tutorials-understand-rolling-tags-containers-index.html)
- [LinuxServer.io Tag Conventions](https://www.linuxserver.io/blog/docker-tags-so-many-tags-so-little-time)
