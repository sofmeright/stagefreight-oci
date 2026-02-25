# StageFreight — Component Configuration

Complete reference for the `component:` block in `.stagefreight.yml`.

Manages GitLab CI component documentation generation and catalog integration.

---

## Top-Level Fields

```yaml
component:
  # Paths to GitLab CI component spec YAML files
  spec_files:
    - "templates/build.yml"
    - "templates/deploy.yml"

  # README injection settings
  readme:
    file: "README.md"                  # target file (default: README.md)
    section: "component-inputs"        # sf:section name (default: "component-inputs")
    branch: "main"                     # branch for --commit (default: main)

  # Add GitLab Catalog link to releases (default: true)
  catalog: true
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec_files` | list of string | `[]` | Paths to component spec YAML files |
| `readme` | object | see below | README documentation injection |
| `catalog` | bool | `true` | Add GitLab Catalog link to releases |

---

## README Injection

Generated documentation is injected into `<!-- sf:component-inputs -->` /
`<!-- /sf:component-inputs -->` markers using the standard narrator section
infrastructure (`registry.ReplaceSection()`).

```yaml
  readme:
    file: "README.md"                  # default
    section: "component-inputs"        # default
    branch: "main"                     # default
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `file` | string | `README.md` | Target README file |
| `section` | string | `component-inputs` | `<!-- sf:<section> -->` marker name |
| `branch` | string | `main` | Branch to commit to (used with `--commit`) |

---

## Spec File Metadata

Component spec files support custom group metadata via YAML comments:

```yaml
# input_section_name- Build Configuration
# input_section_desc- Settings for the container build step

spec:
  inputs:
    registry:
      type: string
      default: "docker.io"
      description: "Container registry URL"
    tag:
      type: string
      description: "Image tag"
```

---

## CLI Commands

### `component docs`

Parse component spec files and generate markdown documentation tables.

```
stagefreight component docs [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--spec` | string slice | from config | Component spec file(s) to parse (repeatable) |
| `--readme` | string | — | Inject docs into this README file |
| `-o`, `--output` | string | — | Write docs to file |
| `--commit` | bool | `false` | Commit updated README via forge API |
| `--branch` | string | from config | Branch to commit to |

### Output Modes

| Mode | Description |
|------|-------------|
| Default | Print markdown to stdout |
| `--readme` | Inject between `<!-- sf:component-inputs -->` markers in README |
| `--output` | Write markdown to specified file |
| `--commit` | Inject into README and commit via forge API (no local clone needed) |
