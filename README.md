# StageFreight (OCI)

> A general-purpose DevOps automation image built to accelerate CI/CD pipelines.  
> Optimized for use with the [StageFreight GitLab component](https://gitlab.prplanit.com/components/stagefreight), but designed to grow beyond GitLab-specific tooling.

---

## Overview

**StageFreight (OCI)** is a Docker image designed to serve as a flexible, prebuilt environment for running infrastructure-as-code and DevOps automation pipelines — especially where fast runtime and broad tooling support are essential.

It is built with a **GitLab-first** mindset but with a clear path toward **platform independence** (e.g., GitHub Actions, Gitea CI, Forgejo, and other CI/CD platforms). While the image is currently used internally by the StageFreight GitLab component, it is being developed into a standalone DevOps utility image.

## Progress Notes
> We are yet to release basic component/docker release management features. Our logics were duplicated accross multiple repositories. We are working towards implementing these features before we can say we even are at an initial release.

Progress:
- ✅ Docker release management - Working but some further optimization can be done.
- 🤷🏽‍♀️ Component release management - We have it working smoothly and learned a lot about the logistics of throwing around scripts and assets that live alongside a component. We are not sure if the syntax issue we got early on with our component inputs grouping setup will work like it did in external testing yet.
- 🚫 Binary (deb/exe/etc.) release management ~ We actually have a project this is done in we forgot about so there is code to be recycled. But we will need time to implement.

---

## Key Features

- ✅ **Preinstalled DevOps toolchain**: Includes `bash`, `curl`, `git`, `jq`, `yq`, `coreutils`, and other essential utilities — ready out of the box.
- ⚡ **Zero bootstrapping time**: Skip installing dependencies in your `before_script`; everything is already available.
- 🧩 **Tailored for CI jobs**: Ideal for release workflows, documentation generation, templating, and file patching tasks.
- 🔄 **Integrated with StageFreight GitLab component**: Used as the base image for jobs like release note generation, README injection, and badge updates.
- 🔧 **Flexible beyond GitLab**: Designed to eventually support other CI platforms by encapsulating logic in portable scripts and tools.
- 🐳 **Built as a foundation image**: Intended for both direct usage and extension into more specialized CI/CD containers.

> ❗ **Note:** For Ansible-specific automation (playbook runs, inventory management, etc.), we recommend using our dedicated image:  
> [`ansible-oci`](https://gitlab.prplanit.com/precisionplanit/ansible-oci)

---

## Use Cases

| Use Case | Description |
|----------|-------------|
| 🔖 GitLab Release Management | Used by the StageFreight component to generate release notes, create releases, and update release metadata |
| 📝 Markdown & README Automation | Injects dynamic tables and documentation into component READMEs |
| 🧪 CI/CD Utility Image | Drop-in toolset for scripting, patching, and infrastructure validation |
| 🔄 Badge Generation | Used to build and push dynamic SVG badges for component health & release status |
| 🚀 Platform-Agnostic Extension | Future target is to ship core tools for release workflows across GitHub, Gitea, Forgejo, etc. |
 
 ---

## See Also
- [Ansible (Gitlab Component)](https://gitlab.prplanit.com/components/ansible)
- [Ansible OCI](https://gitlab.prplanit.com/precisionplanit/ansible-oci) – Docker runtime image for Ansible workflows
- [StageFreight GitLab Component](https://gitlab.prplanit.com/components/stagefreight) – GitLab component that provides CI pipeline orchestration for releases

---

## Image Scope

### Included:

- ✅ General DevOps tools (`bash`, `curl`, `git`, `jq`, `yq`, `coreutils`, etc.)
- ✅ Markdown formatting and file manipulation utilities
- ✅ Portable release automation scripts (e.g. changelog, badge generation)
- ✅ Compatibility with the [StageFreight GitLab Component](#)

### Excluded:

- ❌ Ansible and related modules (see [ansible-oci](https://gitlab.prplanit.com/precisionplanit/ansible-oci))
- ❌ CI platform dependencies (i.e., it does not install GitLab Runner, GitHub Actions CLI, etc.)

---

## Example `.gitlab-ci.yml` Usage

```yaml
generate_release_notes:
  image: registry.prplanit.com/tools/stagefreight:latest
  script:
    - ./scripts/gitlab/generate-release_notes.sh > release.md
```

## Roadmap & Vision
- 🛠 Migrate all component-side tooling from GitLab CI jobs into this image (self-contained workflows)
- 🌐 Provide scripts and CLIs that are CI/CD platform-agnostic
- 🧰 Make it easy to use StageFreight’s features in GitHub Actions, Forgejo Pipelines, Drone CI, and more
- 📦 Package the image for distribution via container registries (e.g., Docker Hub, GHCR)

## Contributing
- If you'd like to contribute tools, scripts, or improvements:
- Fork the repository at StageFreight OCI
- Submit Merge Requests
- Open issues with ideas, bugs, or feature requests

## License
- Distributed under the MIT License.