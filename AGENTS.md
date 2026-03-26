# suisou

A Docker Compose-based sandbox environment for [OpenClaw](https://openclaw.ai/) AI agents.

Unlike similar projects (e.g., OpenShell in `references/`), suisou favors simplicity by relying on familiar Docker tooling for interacting with sandbox containers.

## Architecture

The environment is defined in `compose.yml` with the following services:

- **openclaw** — the OpenClaw AI agent container. Lives on the `sandbox` (internal) network only; all external traffic goes through the router.
- **router** — a mitmproxy-based proxy that mediates the agent's internet access. It enforces a service-based domain + HTTP-method allowlist (`router/config.toml`) and replaces `SUISOU__*` credential markers in outbound requests with real values from its own environment.
- **gateway-proxy** — a socat TCP forwarder that exposes the openclaw gateway (port `18789`) to the host, since the openclaw container has no external network access.

## Documentation

- [AGENTS-developer.md](AGENTS-developer.md) — contributor guide (version control, project layout, coding style).
- [AGENTS-user.md](AGENTS-user.md) — user guide (setup and usage).
