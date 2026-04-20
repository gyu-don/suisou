# suisou

A Docker Compose-based sandbox environment for [OpenClaw](https://openclaw.ai/) AI agents.

Unlike similar projects (e.g., OpenShell in `references/`), suisou favors simplicity by relying on familiar Docker tooling for interacting with sandbox containers.

## Architecture

The environment is defined in `compose.yaml` with the following services:

- **openclaw** — the OpenClaw AI agent container. Shares the `wg-client` network namespace (`network_mode: "service:wg-client"`), so all its traffic flows through the WireGuard tunnel.
- **wg-client** — a WireGuard client that establishes a tunnel to the router and enforces a kill-switch via iptables. The kill-switch blocks all outbound traffic except through the tunnel, preventing the agent from bypassing the proxy. The openclaw container has no `NET_ADMIN` capability, so it cannot alter these firewall rules.
- **router** — Go-based transparent proxy with an embedded userspace WireGuard server (wireguard-go + gvisor netstack). Terminates the WireGuard tunnel, intercepts all HTTP/HTTPS traffic via TLS MITM, and enforces a service-based domain + HTTP-method allowlist (`router/config.toml`). Replaces `SUISOU__*` credential markers in outbound requests with real values from its own environment.

## Documentation

- [AGENTS-developer.md](AGENTS-developer.md) — contributor guide (version control, project layout, coding style).
- [AGENTS-user.md](AGENTS-user.md) — user guide (setup and usage).
