#!/bin/bash
set -eu

CONF="/mitmproxy-ca/wireguard.conf"

echo "Waiting for WireGuard configuration..."
while [ ! -f "$CONF" ]; do sleep 0.5; done

# Parse mitmproxy's JSON config.
# Both values are private keys — derive the server's public key with wg pubkey.
SERVER_PRIVKEY=$(sed -n 's/.*"server_key"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$CONF")
CLIENT_KEY=$(sed -n 's/.*"client_key"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$CONF")
SERVER_PUBKEY=$(echo "$SERVER_PRIVKEY" | wg pubkey)

# mitmproxy WireGuard mode: client address is always 10.0.0.1/32
ADDRESS="10.0.0.1/32"

# Resolve router IP before changing routing
ROUTER_IP=$(getent hosts router | awk '{print $1}')
GATEWAY=$(ip route show default | awk '{print $3}')

# Create WireGuard interface
ip link add wg0 type wireguard
ip addr add "$ADDRESS" dev wg0

KEY_FILE=$(mktemp)
chmod 600 "$KEY_FILE"
echo "$CLIENT_KEY" > "$KEY_FILE"
wg set wg0 \
  private-key "$KEY_FILE" \
  peer "$SERVER_PUBKEY" \
  allowed-ips "0.0.0.0/0" \
  endpoint "${ROUTER_IP}:51820" \
  persistent-keepalive 25
rm "$KEY_FILE"

ip link set wg0 up

# Routing: keep WireGuard handshake via eth0, route everything else through tunnel
ip route add "${ROUTER_IP}/32" via "$GATEWAY"
ip route replace default dev wg0

# Kill-switch: block all outbound traffic not going through the tunnel.
# ESTABLISHED,RELATED on OUTPUT allows response packets for inbound connections
# (e.g. the openclaw gateway on port 18789).
iptables -A OUTPUT -o lo -j ACCEPT
iptables -A OUTPUT -o wg0 -j ACCEPT
iptables -A OUTPUT -p udp -d "${ROUTER_IP}" --dport 51820 -j ACCEPT
iptables -A OUTPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
iptables -P OUTPUT DROP

touch /tmp/wg-ready
echo "WireGuard tunnel ready (${ADDRESS} -> ${ROUTER_IP}:51820)"

exec sleep infinity
