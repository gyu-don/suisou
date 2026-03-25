# User Guide

For detailed OpenClaw documentation, see <https://openclaw.ai/> and `references/openclaw/`.

## Prerequisites

- Docker and Docker Compose

## Quick Start

```sh
docker compose up
```

## Configuration

### `router/allowlist.json`

Controls which domains and HTTP methods the agent can access:

```json
{
  "rules": [
    { "domain": "api.anthropic.com", "methods": ["POST"] }
  ]
}
```

### `router/credentials.json`

Maps dummy API keys to real credentials via environment variables. Copy from `credentials.example.json`:

```sh
cp router/credentials.example.json router/credentials.json
```

Each rule's `env` field names the environment variable the router reads at runtime. The file itself contains no secrets and can be committed as-is.

> **Note:** When adding a rule to `credentials.json`, the corresponding environment variable name must also be listed in the router service's `environment:` section in `compose.yml`. Otherwise the variable won't reach the container.

Provide the variables to the router when starting:

```sh
# inline
ANTHROPIC_API_KEY=sk-ant-... docker compose up

# .env file (keep out of git)
echo 'ANTHROPIC_API_KEY=sk-ant-...' >> .env
docker compose up
```

For secrets management, [Doppler](https://docs.doppler.com/docs/cli) and [1Password CLI](https://developer.1password.com/docs/cli/) are recommended:

```sh
# Doppler
doppler run -- docker compose up

# 1Password CLI (.env contains op:// references)
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
