"""mitmproxy addon: service-based allowlist and credential injection."""

import json
import os
import tomllib
from fnmatch import fnmatch
from pathlib import Path

from mitmproxy import ctx, http, websocket

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

    def request(self, flow: http.HTTPFlow) -> None:
        host = flow.request.pretty_host
        method = flow.request.method.upper()

        # --- allowlist check ---
        allowed = False
        for ep in self.endpoints:
            if fnmatch(host, ep["domain"]):
                methods = [m.upper() for m in ep.get("methods", [])]
                if not methods or method in methods:
                    paths = ep.get("paths", [])
                    if not paths or any(
                        fnmatch(flow.request.path, p) for p in paths
                    ):
                        allowed = True
                if allowed:
                    break

        if not allowed:
            flow.response = http.Response.make(
                403,
                f"Blocked by suisou allowlist: {method} {host}",
                {"Content-Type": "text/plain"},
            )
            return

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
                prefix = rule.get("prefix", "")
                flow.request.headers[header] = current.replace(
                    marker, prefix + real
                )
            else:
                ctx.log.warn(
                    f"suisou: env var {rule['env']!r} not set for {host}"
                )


    def websocket_message(self, flow: http.HTTPFlow) -> None:
        """Inject credentials into WebSocket message payloads.

        Handles Discord Gateway IDENTIFY (op 2) where the bot token is sent
        inside the JSON payload rather than an HTTP header.
        """
        assert flow.websocket is not None
        msg = flow.websocket.messages[-1]
        if msg.from_client and msg.is_text:
            content = msg.text
            if DUMMY_PREFIX not in content:
                return
            try:
                data = json.loads(content)
            except json.JSONDecodeError:
                return
            replaced = self._replace_markers(content)
            if replaced != content:
                msg.text = replaced
                ctx.log.info(
                    f"suisou: injected credentials in WebSocket message "
                    f"(host={flow.request.pretty_host})"
                )

    @staticmethod
    def _replace_markers(text: str) -> str:
        """Replace all SUISOU__<ENV> markers in a string with real values."""
        import re

        def _sub(m: re.Match) -> str:
            env_name = m.group(1)
            return os.environ.get(env_name, m.group(0))

        return re.sub(rf"{DUMMY_PREFIX}(\w+)", _sub, text)


addons = [SuisouAddon()]
