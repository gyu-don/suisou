# suisou

A Docker Compose-based sandbox environment for [OpenClaw](https://openclaw.ai/) AI agents.

Unlike similar projects (e.g., OpenShell in `references/`), suisou favors simplicity by relying on familiar Docker tooling for interacting with sandbox containers.

## Architecture

The environment is defined in `compose.yml` with the following services:

- **openclaw** — the OpenClaw AI agent container.
- **router** — a mitmproxy-based proxy that mediates the agent's internet access. It enforces a service-based domain + HTTP-method allowlist (`router/config.toml`) and replaces `SUISOU__*` credential markers in outbound requests with real values from its own environment.

## Documentation

- [AGENTS-developer.md](AGENTS-developer.md) — contributor guide (build, test, commit conventions).
- [AGENTS-user.md](AGENTS-user.md) — user guide (setup and usage).
