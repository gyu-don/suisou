# User Guide

For detailed OpenClaw documentation, see <https://openclaw.ai/> and `references/openclaw/`.

## Prerequisites

- Docker and Docker Compose
- `jq` (for extracting the gateway token)
- Linux kernel 5.6+ (for built-in WireGuard support)

## Quick Start

```sh
cp router/config.example.toml router/config.toml
```

Run onboard once to configure the gateway (see [official docs](https://docs.openclaw.ai/install/docker.md) for details):

```sh
docker compose build
docker compose run --rm openclaw node openclaw.mjs onboard \
  --mode local --no-install-daemon
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

The router uses a naming convention to replace dummy credentials with real ones. This requires settings in two places — `compose.override.yml` configures both together:

```yaml
# compose.override.yml
services:
  openclaw:
    environment:
      # Sandbox sees only the marker, never the real key
      - ANTHROPIC_API_KEY=SUISOU__ANTHROPIC_API_KEY
  router:
    environment:
      # Router receives the real key from the host environment
      - ANTHROPIC_API_KEY
```

When the router sees `SUISOU__ANTHROPIC_API_KEY` in an outbound HTTP header, it replaces it with the real `ANTHROPIC_API_KEY` from its own environment. The matching header is defined in `router/config.toml` under `[services.<name>.credentials]`.

### `compose.override.yml`

Optional. Use this for per-user Docker Compose overrides (credentials, extra services, etc.). Docker Compose automatically merges this with `compose.yml`.

```sh
cp compose.override.example.yml compose.override.yml
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

### wg-client

Establishes a WireGuard tunnel to the router and enforces an iptables kill-switch that blocks all outbound traffic except through the tunnel. The openclaw container shares this network namespace but has no `NET_ADMIN` capability, so it cannot alter the firewall rules.

### Moltbook (read-only)

[Moltbook](https://www.moltbook.com/) is an AI agent social network. All endpoints require an API key — anonymous reads are not possible.

**Step 1 — Register the agent (one-time, outside the sandbox)**

```sh
curl -s -X POST https://www.moltbook.com/api/v1/agents/register \
  -H "Content-Type: application/json" \
  -d '{"name": "YOUR_AGENT_NAME", "description": "YOUR_DESCRIPTION"}' | jq .
```

Save the returned `api_key`. The response also contains a `claim_url` and `verification_code`.

**Step 2 — Claim the account**

Open the `claim_url` in a browser, verify your email, then post the `verification_code` to X (Twitter). Moltbook checks the tweet to confirm ownership. The account becomes active once verified.

**Step 3 — Store the API key in Doppler**

```sh
doppler secrets set MOLTBOOK_API_KEY=<api_key from step 1>
```

**Step 4 — Configure credential injection**

Add to `compose.override.yml`:

```yaml
services:
  openclaw:
    environment:
      - MOLTBOOK_API_KEY=SUISOU__MOLTBOOK_API_KEY
  router:
    environment:
      - MOLTBOOK_API_KEY
```

**Step 5 — Add the service to `router/config.toml`**

Uncomment the Moltbook block (see `router/config.example.toml`). Only `GET` is allowed, so the agent can read feeds, posts, comments, profiles, and search results but cannot post, comment, vote, follow, or modify anything.

> **Note:** Always use `www.moltbook.com` (with `www`). Without it, the server redirects and strips the Authorization header, exposing a broken request.

### router

Proxies and controls the agent's outbound internet access via mitmproxy in WireGuard mode.

- All traffic from the agent is transparently intercepted through the WireGuard tunnel — no `HTTP_PROXY` configuration required.
- Allows or blocks requests per domain and HTTP method via a service-based allowlist.
- For configured services, transparently replaces `SUISOU__*` credential markers in outbound requests with real values.
