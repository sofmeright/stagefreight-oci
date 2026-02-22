# `.stagefreight.yml` Example Manifests

Example configurations for every project archetype. Each file is a standalone, copy-paste-ready `.stagefreight.yml` with comments explaining who it's for, what it does, and why.

## Per-Repo Manifests (`.stagefreight.yml`)

| # | Example | Who it's for |
|---|---------|-------------|
| 01 | [minimal](01-minimal.yml) | Solo dev, hobby project — release notes + badge, nothing else |
| 02 | [personal-docker-app](02-personal-docker-app.yml) | Solo dev shipping a containerized side project with dev environment |
| 03 | [patched-fork](03-patched-fork.yml) | Anyone maintaining local patches on top of upstream |
| 04 | [mirror](04-mirror.yml) | Read-only mirror of a source-of-truth elsewhere |
| 05 | [multi-image](05-multi-image.yml) | Monolithic codebase producing app + worker + migrations from one Dockerfile |
| 06 | [cross-platform-cli](06-cross-platform-cli.yml) | CLI binary for multiple OS/arch — no container image, artifact builds only |
| 07 | [multi-toolchain-build](07-multi-toolchain-build.yml) | Complex build needing multiple SDKs (Go + .NET, Rust + C) |
| 08 | [full-lifecycle](08-full-lifecycle.yml) | Business app with everything turned on — multi-registry, compliance, the works |
| 09 | [internal-tool](09-internal-tool.yml) | Private team tooling — single registry, no public push |
| 10 | [library-package](10-library-package.yml) | npm/PyPI/Go library — no Docker, published to package registries |
| 11 | [static-site](11-static-site.yml) | Documentation repo or static site — link checking, flair, no build |
| 12 | [homelab-fork](12-homelab-fork.yml) | Homelab user with a customized fork — auto-sync, auto-rebuild, YOLO |
| 13 | [compliance-strict](13-compliance-strict.yml) | Regulated industry — all security mandatory, SBOM required, no YOLO |
| 14 | [microservices-monorepo](14-microservices-monorepo.yml) | Multiple distinct services in one repo, each with own Dockerfile |
| 15 | [headless-worker](15-headless-worker.yml) | Queue consumer / bot / cron job — no HTTP, process-level testing |
| 16 | [helm-chart](16-helm-chart.yml) | Helm chart or IaC module — lint, template, package, publish |
| 17 | [game-server](17-game-server.yml) | Modded game server — fork of upstream, custom mods, YOLO |
| 18 | [mobile-hybrid-app](18-mobile-hybrid-app.yml) | Mobile/hybrid app — artifact builds for APK/web + container for API |
| 19 | [open-source-maintainer](19-open-source-maintainer.yml) | OSS project — every registry, public SBOM, community workflow |
| 20 | [data-pipeline](20-data-pipeline.yml) | ETL / data pipeline — mock data stack, pipeline validation |
| 21 | [ml-model](21-ml-model.yml) | ML model serving — GPU image + ONNX model artifact export |
| 22 | [s6-overlay-linuxserver](22-s6-overlay-linuxserver.yml) | S6-overlay / LinuxServer.io style image — LSCR tags, PUID/PGID |
| 23 | [ansible-collection](23-ansible-collection.yml) | Ansible collection/role — molecule testing, Galaxy publishing |

## Daemon Configuration

| # | Example | What it is |
|---|---------|-----------|
| 24 | [daemon-config](24-daemon-config.yml) | `stagefreight serve` config — providers, org defaults, mirrors, alerts |

## Manifest Complexity Gradient

```
4 lines    01-minimal              ← just release notes + badge
~25 lines  04-mirror, 11-static   ← declaration + light features
~40 lines  02-personal, 09-internal, 10-library  ← single build + dev env
~60 lines  03-fork, 06-cli, 15-worker, 16-helm   ← focused use cases
~80 lines  05-multi-image, 07-multi-toolchain     ← complex builds
~100 lines 08-full, 13-compliance, 14-microservices, 19-oss  ← everything on
```
