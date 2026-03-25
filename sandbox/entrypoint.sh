#!/bin/bash
set -eu

# --------------------------------------------------------------------------
# Install mitmproxy CA certificate (requires root)
# --------------------------------------------------------------------------
CA_SRC="/mitmproxy-ca/mitmproxy-ca-cert.pem"
CA_DST="/usr/local/share/ca-certificates/mitmproxy-ca.crt"
for i in $(seq 1 30); do
  [ -f "$CA_SRC" ] && break
  sleep 0.5
done
if [ -f "$CA_SRC" ]; then
  cp "$CA_SRC" "$CA_DST"
  update-ca-certificates --fresh > /dev/null 2>&1
  # Node.js needs this for TLS verification through the proxy
  export NODE_EXTRA_CA_CERTS="$CA_DST"
fi

# --------------------------------------------------------------------------
# Configure openclaw for ollama (first run only)
# --------------------------------------------------------------------------
CONFIG_DIR="/home/node/.openclaw"
chown -R node:node "$CONFIG_DIR" 2>/dev/null || true
if [ ! -f "$CONFIG_DIR/openclaw.json" ]; then
  gosu node openclaw onboard \
    --non-interactive \
    --accept-risk \
    --mode local \
    --no-install-daemon \
    --skip-skills \
    --skip-health \
    --auth-choice custom-api-key \
    --custom-base-url "${OLLAMA_BASE_URL:-http://ollama:11434/v1}" \
    --custom-model-id "${OLLAMA_MODEL:-nemotron-cascade-2:30b}" \
    --custom-api-key "ollama" \
    --secret-input-mode plaintext \
    --custom-compatibility openai \
    --gateway-port 18789 \
    --gateway-bind lan
fi

# --------------------------------------------------------------------------
# Start gateway as non-root node user
# --------------------------------------------------------------------------
exec gosu node "$@"
