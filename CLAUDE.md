- Repository Information:
  - Git repository: https://gitlab.prplanit.com/sofmeright/stagefreight
  - **Git Commits**: Only sign commits as sofmeright@gmail.com / SoFMeRight. No anthropic attribution comments in commits.

- CRITICAL RULES:
  - STAY ON TASK when following directions. NO BAND AID, NO WORK AROUNDS. If you think we need to give up or regroup, ASK. Don't make the call on your own to find alternative solutions or shortcuts.
  - I want things done exactly how I ask. If you want to offer an alternative, conversation should stop till I tell you if I agree/disagree.

- Building & Testing:
  - **StageFreight builds itself (dogfood).** Use the previous release image to build:
    ```bash
    docker run --rm -v "$PWD":/src -w /src -v /var/run/docker.sock:/var/run/docker.sock \
      docker.io/prplanit/stagefreight:0.2.0-alpha.2 stagefreight docker build --local
    ```
  - `--dry-run` to verify plan resolution without building. `--local` to load into daemon without pushing.
  - **CI pipeline** (`.gitlab-ci.yml`): Same dogfood approach — CI image is the previous release. `stagefreight docker build` handles detect → lint → plan → build → push → retention.

- Architecture:
  - Go CLI at `src/cli/main.go`, commands under `src/cli/cmd/`
  - Build engine: `src/build/` (plan, buildx, tags, version detection)
  - Registry providers: `src/registry/` (dockerhub, ghcr, gitlab, quay, jfrog, harbor, gitea, local)
  - Forge abstraction: `src/forge/` (GitLab, GitHub, Gitea/Forgejo)
  - Lint engine: `src/lint/` with modules under `src/lint/modules/`
  - Config: `src/config/` — parsed from `.stagefreight.yml`

- Build Strategy:
  - **Single-platform**: `--load` into daemon → `docker push` each remote tag. Image exists locally AND remotely. Both local and remote retention work.
  - **Multi-platform**: `--push` directly (buildx limitation). No local copy. Remote retention only.
  - `provider: local` registries: images stay in the Docker daemon only. Retention prunes via `docker rmi`.

- Retention:
  - Restic-style additive policies: `keep_last`, `keep_daily`, `keep_weekly`, `keep_monthly`, `keep_yearly`
  - A tag survives if ANY policy wants to keep it
  - Config accepts integer shorthand (`retention: 10` = keep_last: 10) or full policy map
  - Tag patterns (from `tags:` field) are converted to regex for matching which remote tags are retention candidates

- Input Validation:
  - Registry URLs, image paths, tags, credentials, provider names, and regex patterns are all validated at plan time
  - Resolved tags are validated against OCI spec before any push
  - Fail fast with clear errors, not cryptic Docker failures
