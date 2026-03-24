# suisou

A Docker Compose-based sandbox environment for [OpenClaw](https://openclaw.ai/) AI agents.

Unlike similar projects (e.g., OpenShell in `references/`), suisou favors simplicity by relying on familiar Docker tooling for interacting with sandbox containers.

## Architecture

The environment is defined in `compose.yml` with the following services:

- **openclaw** — the OpenClaw AI agent container.
- **router** — a proxy (likely mitmproxy) that mediates the agent's internet access. It enforces a domain + HTTP-method allowlist and rewrites dummy API keys/tokens to real credentials for designated domains.
- **inference** *(optional)* — a [Bifrost](https://docs.getbifrost.ai/overview)-based LLM gateway. Useful when serving local models; for cloud-only setups, direct HTTPS through the router suffices.

## Documentation

- [AGENTS-developer.md](AGENTS-developer.md) — contributor guide (build, test, commit conventions).
- [AGENTS-user.md](AGENTS-user.md) — user guide (setup and usage).
