# User Guide

For detailed OpenClaw documentation, see <https://openclaw.ai/> and `references/openclaw/`.

## Prerequisites

- Docker and Docker Compose

## Quick Start

```sh
cp router/config.example.toml router/config.toml
```

Run onboard once to configure the gateway (see [official docs](https://docs.openclaw.ai/install/docker.md) for details):

```sh
docker compose build
docker compose run --rm --no-deps --entrypoint node openclaw \
  openclaw.mjs onboard --mode local --no-install-daemon
```

Then start the services:

```sh
docker compose up
```

Edit `router/config.toml` to match your needs. The example file shows the available format.

## Configuration

### `router/config.toml`

Controls which domains the agent can access and how credentials are injected. Copy from the example and edit:

```sh
cp router/config.example.toml router/config.toml
```

See `router/config.example.toml` for the full format.

### Credential injection

The router uses a naming convention to replace dummy credentials with real ones. In the sandbox, set environment variables to the literal marker `SUISOU__<ENV_NAME>`:

```yaml
# compose.override.yml
services:
  openclaw:
    environment:
      - ANTHROPIC_API_KEY=SUISOU__ANTHROPIC_API_KEY
```

When the router sees `SUISOU__ANTHROPIC_API_KEY` in the configured HTTP header, it replaces it with the real value from its own environment.

### `compose.override.yml`

Optional. Use this to pass environment variables to containers, or any other per-user Docker Compose overrides. Docker Compose automatically merges this with `compose.yml`.

```sh
cp compose.override.example.yml compose.override.yml
```

The router container needs the real API keys passed through:

```yaml
services:
  router:
    environment:
      - ANTHROPIC_API_KEY
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

## Remote Access

The gateway listens on port `18789`. To connect from another machine, forward the port over SSH:

```sh
ssh -L 18789:localhost:18789 <user>@<host>
```

Then open `http://localhost:18789/` in a browser.

### Gateway token

After starting the services, retrieve the gateway token:

```sh
docker compose exec openclaw sh -c \
  "cat /home/node/.openclaw/openclaw.json" | jq -r '.gateway.auth.token'
```

Paste the token into the **Gateway Token** field on the Control UI login screen.

### Device pairing

When connecting from a new browser, OpenClaw requires device pairing approval. Approve from the host:

```sh
# Show pending requests
docker compose exec openclaw cat /home/node/.openclaw/devices/pending.json

# Approve via the CLI (requires gateway.remote.token config)
docker compose exec openclaw openclaw devices approve <request-id>
```

## Services

### openclaw

The AI agent container. Interact with it using standard Docker commands:

```sh
docker compose exec openclaw bash
```

### router

Proxies and controls the agent's outbound internet access.

- Allows or blocks requests per domain and HTTP method via a service-based allowlist.
- For configured services, transparently replaces `SUISOU__*` credential markers in outbound requests with real values.
