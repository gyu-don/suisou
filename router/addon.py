"""mitmproxy addon: service-based allowlist and credential injection."""

import json
import os
import tomllib
from fnmatch import fnmatch
from pathlib import Path

from mitmproxy import ctx, http, tcp, udp

CONFIG_PATH = Path("/etc/suisou/config.toml")
DUMMY_PREFIX = "SUISOU__"


def _load_config(path: Path) -> dict:
    if not path.exists():
        return {}
    return tomllib.loads(path.read_text())


class SuisouAddon:
    def __init__(self) -> None:
        self.endpoints: list[dict] = []
        self.credentials: list[dict] = []

    def load(self, loader):  # noqa: ARG002
        config = _load_config(CONFIG_PATH)

        for svc_name, svc in config.get("services", {}).items():
            cred = svc.get("credentials")
            for ep in svc.get("endpoints", []):
                self.endpoints.append(ep)
                if cred:
                    self.credentials.append({**cred, "domain": ep["domain"]})

            ctx.log.info(
                f"suisou: loaded service {svc_name!r} "
                f"({len(svc.get('endpoints', []))} endpoints)"
            )

        ctx.log.info(
            f"suisou: {len(self.endpoints)} endpoints, "
            f"{len(self.credentials)} credential rules"
        )

    def _get_host(self, flow: http.HTTPFlow) -> str | None:
        """Return the logical hostname for allowlist matching.

        Uses ``pretty_host`` (Host / :authority header) for domain matching
        because ``flow.request.host`` returns the raw destination IP in
        WireGuard transparent proxy mode.

        To prevent Host-header spoofing (a client claiming an allowed domain
        while connecting to a blocked destination):

        - **TLS**: the SNI — set at the transport layer and verified against
          the upstream certificate by mitmproxy — must match ``pretty_host``.
        - **Plain HTTP**: ``pretty_host`` is trusted only when the matching
          endpoint has ``allow_plain_http = true`` in config.  Without that
          flag, plain-HTTP requests are blocked because there is no
          transport-level signal to verify the Host header.
        """
        host = flow.request.pretty_host
        if flow.client_conn.tls_established:
            sni = flow.client_conn.sni
            if sni and sni != host:
                ctx.log.warn(
                    f"suisou: Host/SNI mismatch: "
                    f"Host={host!r}, SNI={sni!r} — blocking"
                )
                return None
        else:
            if not self._plain_http_allowed(host):
                ctx.log.warn(
                    f"suisou: plain HTTP not allowed for {host!r} — blocking"
                )
                return None
        return host

    def _plain_http_allowed(self, host: str) -> bool:
        return any(
            ep.get("allow_plain_http", False)
            for ep in self.endpoints
            if fnmatch(host, ep["domain"])
        )

    @staticmethod
    def _get_port(flow: http.HTTPFlow) -> int:
        return int(flow.request.port)

    @staticmethod
    def _endpoint_matches(
        ep: dict, host: str, method: str, path: str, port: int
    ) -> bool:
        if not fnmatch(host, ep["domain"]):
            return False

        methods = [m.upper() for m in ep.get("methods", [])]
        if methods and method not in methods:
            return False

        paths = ep.get("paths", [])
        if paths and not any(fnmatch(path, pattern) for pattern in paths):
            return False

        ports = ep.get("ports", [])
        if ports and port not in [int(p) for p in ports]:
            return False

        return True

    def _format_target(self, flow: http.HTTPFlow) -> str:
        host = flow.request.pretty_host
        port = self._get_port(flow)
        path = flow.request.path or "/"
        return f"{host}:{port}{path}"

    def request(self, flow: http.HTTPFlow) -> None:
        host = self._get_host(flow)
        if host is None:
            flow.response = http.Response.make(
                403,
                "Blocked by suisou: untrusted Host header",
                {"Content-Type": "text/plain"},
            )
            return
        method = flow.request.method.upper()
        path = flow.request.path or "/"
        port = self._get_port(flow)

        # --- allowlist check ---
        allowed = any(
            self._endpoint_matches(ep, host, method, path, port)
            for ep in self.endpoints
        )

        if not allowed:
            ctx.log.warn(
                "suisou: blocked request "
                f"({method} {self._format_target(flow)})"
            )
            flow.response = http.Response.make(
                403,
                f"Blocked by suisou allowlist: {method} {host}:{port}",
                {"Content-Type": "text/plain"},
            )
            return

        ctx.log.info(
            f"suisou: allowed request ({method} {self._format_target(flow)})"
        )

        # --- credential injection ---
        for rule in self.credentials:
            if not fnmatch(host, rule["domain"]):
                continue
            header = rule["header"]
            current = flow.request.headers.get(header, "")
            marker = DUMMY_PREFIX + rule["env"]
            if marker not in current:
                continue
            real = os.environ.get(rule["env"], "")
            if real:
                flow.request.headers[header] = current.replace(marker, real)
            else:
                ctx.log.warn(
                    f"suisou: env var {rule['env']!r} not set for {host}"
                )

    def tcp_start(self, flow: tcp.TCPFlow) -> None:
        """Block all non-HTTP TCP connections."""
        dest = flow.server_conn.address
        ctx.log.warn(
            f"suisou: blocked non-HTTP TCP connection to {dest[0]}:{dest[1]}"
        )
        flow.kill()

    def udp_start(self, flow: udp.UDPFlow) -> None:
        """Block all non-DNS UDP connections.

        DNS (port 53) is handled by mitmproxy's built-in resolver and does
        not surface as a UDPFlow, so everything that reaches this hook is
        non-DNS and should be blocked.
        """
        dest = flow.server_conn.address
        ctx.log.warn(
            f"suisou: blocked UDP connection to {dest[0]}:{dest[1]}"
        )
        flow.kill()

    def websocket_start(self, flow: http.HTTPFlow) -> None:
        flow.metadata["ws_client_messages"] = 0
        flow.metadata["ws_server_messages"] = 0
        flow.metadata["ws_injected_messages"] = 0
        ctx.log.info(
            f"suisou: websocket opened ({self._format_target(flow)})"
        )

    def websocket_message(self, flow: http.HTTPFlow) -> None:
        """Inject credentials into WebSocket message payloads and count traffic.

        Handles Discord Gateway IDENTIFY (op 2) where the bot token is sent
        inside the JSON payload rather than an HTTP header.
        """
        assert flow.websocket is not None
        msg = flow.websocket.messages[-1]
        if msg.from_client:
            flow.metadata["ws_client_messages"] = (
                int(flow.metadata.get("ws_client_messages", 0)) + 1
            )
        else:
            flow.metadata["ws_server_messages"] = (
                int(flow.metadata.get("ws_server_messages", 0)) + 1
            )
        if msg.from_client and msg.is_text:
            content = msg.text
            if DUMMY_PREFIX not in content:
                return
            try:
                json.loads(content)
            except json.JSONDecodeError:
                return
            host = flow.request.pretty_host
            allowed_envs = {
                rule["env"]
                for rule in self.credentials
                if fnmatch(host, rule["domain"])
            }
            replaced = self._replace_markers(content, allowed_envs)
            if replaced != content:
                msg.text = replaced
                flow.metadata["ws_injected_messages"] = (
                    int(flow.metadata.get("ws_injected_messages", 0)) + 1
                )

    def websocket_end(self, flow: http.HTTPFlow) -> None:
        ctx.log.info(
            "suisou: websocket closed "
            f"({self._format_target(flow)}, "
            f"client_messages={int(flow.metadata.get('ws_client_messages', 0))}, "
            f"server_messages={int(flow.metadata.get('ws_server_messages', 0))}, "
            f"injected_messages={int(flow.metadata.get('ws_injected_messages', 0))})"
        )

    @staticmethod
    def _replace_markers(text: str, allowed_envs: set[str]) -> str:
        """Replace SUISOU__<ENV> markers with real values for allowed env vars only."""
        import re

        def _sub(m: re.Match) -> str:
            env_name = m.group(1)
            if env_name not in allowed_envs:
                return m.group(0)
            return os.environ.get(env_name, m.group(0))

        return re.sub(rf"{DUMMY_PREFIX}(\w+)", _sub, text)


addons = [SuisouAddon()]
