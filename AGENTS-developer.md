# Developer Guide

## Language

Use English for all code, configuration files, and documentation.

## Documentation Principles

- Do not duplicate information that is already expressed in code or configuration files.
- If a doc restates what a file already shows, remove the duplication and point to the file instead.

## Version Control

This project uses [jj](https://martinvonz.github.io/jj/) for version control.

## Project Layout

```
compose.yml          # Service definitions (openclaw, router)
references/          # Related projects (read-only context, not upstream)
  openclaw/          # OpenClaw source — primary reference for usage and integration
  OpenShell/
  OpenShell-Community/
```

The `references/` directory contains related projects as read-only context. `references/openclaw/` is the primary reference — OpenClaw is a very new project, so consult its source for up-to-date usage patterns. The OpenShell projects are structurally different and should not be treated as a template.

## Coding Style

- Keep configuration minimal and explicit.
- Prefer standard Docker / Compose idioms over custom abstractions.
