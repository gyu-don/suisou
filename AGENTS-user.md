# User Guide

For detailed OpenClaw documentation, see <https://openclaw.ai/> and `references/openclaw/`.

## Prerequisites

- Docker and Docker Compose

## Quick Start

```sh
docker compose up
```

To include the inference gateway:

```sh
docker compose --profile inference up
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

Maps dummy API keys to real ones. Copy from `credentials.example.json` and fill in real values:

```sh
cp router/credentials.example.json router/credentials.json
```

### `inference/config.json`

Bifrost provider configuration. Copy from `config.example.json`:

```sh
cp inference/config.example.json inference/config.json
```

See [Bifrost docs](https://docs.getbifrost.ai/overview) for provider setup.

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

### inference (optional)

An LLM inference gateway powered by [Bifrost](https://docs.getbifrost.ai/overview). Accessible from the agent at `http://inference:8080` (no proxy needed).

If you only use cloud-hosted models, you can skip this service and route HTTPS traffic through the router directly.
