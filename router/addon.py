"""mitmproxy addon: domain+method allowlist and credential injection."""

import json
import os
from pathlib import Path

from mitmproxy import ctx, http

ALLOWLIST_PATH = Path("/etc/suisou/allowlist.json")
CREDENTIALS_PATH = Path("/etc/suisou/credentials.json")


def _load_json(path: Path) -> dict:
    if not path.exists():
        return {}
    return json.loads(path.read_text())


class SuisouAddon:
    def __init__(self) -> None:
        self.allowlist_rules: list[dict] = []
        self.credential_rules: list[dict] = []

    def load(self, loader):  # noqa: ARG002
        self.allowlist_rules = _load_json(ALLOWLIST_PATH).get("rules", []) or []
        self.credential_rules = _load_json(CREDENTIALS_PATH).get("rules", []) or []
        ctx.log.info(
            f"suisou: {len(self.allowlist_rules)} allowlist rules, "
            f"{len(self.credential_rules)} credential rules"
        )

    def request(self, flow: http.HTTPFlow) -> None:
        host = flow.request.pretty_host
        method = flow.request.method.upper()

        # --- allowlist check ---
        allowed = False
        for rule in self.allowlist_rules:
            if rule["domain"] == host:
                methods = [m.upper() for m in rule.get("methods", [])]
                if not methods or method in methods:
                    allowed = True
                break

        if not allowed:
            flow.response = http.Response.make(
                403,
                f"Blocked by suisou allowlist: {method} {host}",
                {"Content-Type": "text/plain"},
            )
            return

        # --- credential injection ---
        for rule in self.credential_rules:
            if rule["domain"] != host:
                continue
            header = rule["header"]
            current = flow.request.headers.get(header, "")
            if current == rule["dummy"]:
                real = os.environ.get(rule["env"], "")
                if real:
                    flow.request.headers[header] = real
                else:
                    ctx.log.warn(f"suisou: env var {rule['env']!r} not set for {host}")


addons = [SuisouAddon()]
