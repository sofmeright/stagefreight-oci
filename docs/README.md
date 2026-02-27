# StageFreight Documentation

## Reference

- [Dependency Update](DependencyUpdate.md) — `dependency update` command, Go toolchain strategies, container runner, artifacts
- [Docker Build](Docker.md) — Full `docker:` block schema, registries, cache, retention, build strategy
- [Release Management](Release.md) — Full `release:` block schema, sync targets, rolling tags, CLI commands
- [Security Scanning](Security.md) — Full `security:` block schema, detail levels, scan artifacts
- [Linter Configuration](Linter.md) — Full `lint:` block schema, cache TTL contract, CLI flags
- [Narrator & Badges](Narrator.md) — `badges:`, `docker.readme:`, `narrator:` blocks, managed sections
- [Component Docs](Component.md) — Full `component:` block schema, spec file parsing, CLI commands
- [Configuration Examples](config/README.md) — 24 example `.stagefreight.yml` manifests for every project archetype
- [Known Issues](KnownIssues.md) — Active bugs and workarounds

## Shared Primitives

These structures are used across multiple config blocks:

- **Retention Policy** — Restic-style tag/release cleanup ([reference](Docker.md#retention-policy))
- **Pattern Syntax** — Regex/literal/negated patterns for branches and tags ([reference](Docker.md#pattern-syntax))
- **Condition** — Tag/branch-sensitive rule primitive ([reference](Security.md#condition))
- **Template Variables** — `{version}`, `{env:VAR}`, `{docker.pulls}`, etc. ([reference](Narrator.md#template-variables))

## Planning

- [Road Map](RoadMap.md) — Full product vision, phased feature plan, and [tracking index](RoadMap.md#tracking-index)
  - [Freshness Module](RoadMap.md#freshness-module--versioning--detection) — Tag classification, versioning, planned improvements
  - [Configuration API Redesign](RoadMap.md#freshness-configuration-api-redesign) — Planned extends/adherence/repository schema
  - [Tag Pattern Coverage](RoadMap.md#freshness-tag-pattern-coverage-matrix) — Real-world tag patterns and their status
