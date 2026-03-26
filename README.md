# Suisou

A Docker Compose sandbox for [OpenClaw](https://openclaw.ai/) AI agents with controlled network access and secure credential injection.

The router (mitmproxy) sits between the agent container and the internet, enforcing a domain allowlist and transparently injecting secrets so they never touch the sandbox.

## Quick Start

```sh
# 1. Configure the router allowlist
cp router/config.example.toml router/config.toml
# Edit router/config.toml to add your services

# 2. Create compose.override.yml for credentials (see compose.override.example.yml)
cp compose.override.example.yml compose.override.yml

# 3. Build and onboard
docker compose build
docker compose run --rm --no-deps --entrypoint node openclaw \
  openclaw.mjs onboard --mode local --no-install-daemon

# 4. Start (pass secrets via env, Doppler, 1Password CLI, etc.)
ANTHROPIC_API_KEY=sk-ant-... docker compose up
```

The gateway is available at http://localhost:18789/.

## Documentation

See [AGENTS-user.md](AGENTS-user.md) for setup details and configuration, and [AGENTS.md](AGENTS.md) for architecture overview. AI agents working on this repo should also read [AGENTS-developer.md](AGENTS-developer.md).

## License

[MIT](LICENSE) -- Copyright (c) 2026 gyu-don
