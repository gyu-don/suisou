# User Guide

For detailed OpenClaw documentation, see <https://openclaw.ai/> and `references/openclaw/`.

## Prerequisites

- Docker and Docker Compose

## Quick Start

```sh
cp router/allowlist.example.json router/allowlist.json
cp router/credentials.example.json router/credentials.json
docker compose up
```

Edit the copied files to match your needs. The example files show the available format.

## Configuration

### `router/allowlist.json`

Controls which domains and HTTP methods the agent can access. Copy from the example and edit:

```sh
cp router/allowlist.example.json router/allowlist.json
```

```json
{
  "rules": [
    { "domain": "api.anthropic.com", "methods": ["POST"] }
  ]
}
```

### `router/credentials.json`

Maps dummy API keys to real credentials via environment variables. Copy from the example and edit:

```sh
cp router/credentials.example.json router/credentials.json
```

Each rule's `env` field names the environment variable the router reads at runtime.

### `compose.override.yml`

Optional. Use this to add environment variable passthrough for extra API keys, or any other per-user Docker Compose overrides. Docker Compose automatically merges this with `compose.yml`.

```sh
cp compose.override.example.yml compose.override.yml
```

When adding a rule to `credentials.json`, the corresponding environment variable must be passed to the router container. Add it to `compose.override.yml`:

```yaml
services:
  router:
    environment:
      - GOOGLE_API_KEY
```

### Secrets

Provide API key environment variables when starting. [Doppler](https://docs.doppler.com/docs/cli) is recommended:

```sh
doppler run -- docker compose up
```

Other options:

```sh
# inline
ANTHROPIC_API_KEY=sk-ant-... docker compose up

# 1Password CLI
op run --env-file=.env -- docker compose up
```

## Services

### openclaw

The AI agent container. Interact with it using standard Docker commands:

```sh
docker compose exec openclaw bash
```

### router

Proxies and controls the agent's outbound internet access.

- Allows or blocks requests per domain and HTTP method via an allowlist.
- For configured domains, transparently replaces dummy API keys/tokens in outbound requests with real credentials.
