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

The router uses a naming convention to replace dummy credentials with real ones. This requires settings in two places — `compose.override.yaml` configures both together:

```yaml
# compose.override.yaml
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

### `compose.override.yaml`

Optional. Use this for per-user Docker Compose overrides (credentials, extra services, etc.). Docker Compose automatically merges this with `compose.yaml`.

```sh
cp compose.override.example.yaml compose.override.yaml
```

### Secrets

Provide API key environment variables when starting. [Doppler](https://docs.doppler.com/docs/cli) is recommended.

Install the CLI, run `doppler login` (first time), then `doppler setup` in this directory (once per project). Both are interactive — run them in a regular terminal. See the [Doppler CLI docs](https://docs.doppler.com/docs/install-cli) for install instructions.

#### Setting secrets

Open the dashboard in a browser:

```sh
doppler open
```

Or via CLI — but be aware that any value written in a command is recorded in shell history and, if run through an AI agent, in its session context as well:

```sh
doppler secrets set ANTHROPIC_API_KEY=sk-ant-...
```

#### Running with secrets injected

```sh
doppler run -- docker compose up
```

If you need to specify a project or config explicitly (e.g. in CI):

```sh
doppler run -p PROJECT -c CONFIG -- docker compose up
```

#### Other options

```sh
# 1Password CLI
op run --env-file=.env -- docker compose up
```

Passing secrets inline (e.g. `ANTHROPIC_API_KEY=sk-ant-... docker compose up`) is not recommended — the value ends up in shell history and, if run through an AI agent, in its session context as well.

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

### router

Proxies and controls the agent's outbound internet access via mitmproxy in WireGuard mode.

- All traffic from the agent is transparently intercepted through the WireGuard tunnel — no `HTTP_PROXY` configuration required.
- Allows or blocks requests per domain and HTTP method via a service-based allowlist.
- For configured services, transparently replaces `SUISOU__*` credential markers in outbound requests with real values.

## External service integrations

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

**Step 3 — Store the API key**

Set `MOLTBOOK_API_KEY` to the value returned in step 1, using whichever method you use for secrets (see [Secrets](#secrets)).

**Step 4 — Configure credential injection**

Add to `compose.override.yaml`:

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

### Discord

See the [OpenClaw Discord channel docs](https://docs.openclaw.ai/channels/discord.md) for the full OpenClaw-side configuration. The suisou-specific steps are below.

**Step 1 — Create a Discord bot**

In the [Discord Developer Portal](https://discord.com/developers/applications):

1. Create a new application and add a bot.
2. Under **Bot**, enable the following privileged Gateway Intents:
   - **Message Content Intent** (required)
   - **Server Members Intent** (recommended)
3. Under **OAuth2 → URL Generator**, select the `bot` and `applications.commands` scopes. Assign at minimum: Send Messages, Read Message History, Attach Files.
4. Copy the generated URL, open it in a browser, and invite the bot to your server.

**Step 2 — Store the bot token**

Set `DISCORD_BOT_TOKEN` to the token from the Developer Portal, using whichever method you use for secrets (see [Secrets](#secrets)).

**Step 3 — Configure credential injection**

Add to `compose.override.yaml`:

```yaml
services:
  openclaw:
    environment:
      - DISCORD_BOT_TOKEN=SUISOU__DISCORD_BOT_TOKEN
  router:
    environment:
      - DISCORD_BOT_TOKEN
```

**Step 4 — Add the service to `router/config.toml`**

Uncomment the Discord block (see `router/config.example.toml`). It allows GET/POST/PUT/PATCH/DELETE on `discord.com` (REST API), GET on `cdn.discordapp.com` (attachments), and unrestricted access to `gateway.discord.gg` and `*.discord.gg` (WebSocket gateway).

**Step 5 — Configure the OpenClaw gateway**

Follow the OpenClaw docs to connect the Discord channel to the gateway. The bot token is passed via the environment variable set above.
