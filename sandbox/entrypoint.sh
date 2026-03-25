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
  export NODE_EXTRA_CA_CERTS="$CA_DST"
fi

# --------------------------------------------------------------------------
# Drop to node user and run the official entrypoint
# --------------------------------------------------------------------------
exec gosu node docker-entrypoint.sh "$@"
