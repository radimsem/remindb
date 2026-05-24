"""Hermes Agent memory provider for remindb.

Spawns `remindb serve` as a subprocess and proxies Hermes' tool-calling
lifecycle into MCP `tools/list` / `tools/call` round-trips, speaking
JSON-RPC over the subprocess's stdio pipes.
"""

from __future__ import annotations

import json
import logging
import os
import pathlib
import shutil
import subprocess
import sys
import threading
from typing import Any, Dict, List, Optional

from agent.memory_provider import MemoryProvider

logger = logging.getLogger(__name__)


def _read_plugin_version() -> str:
    """Read the version from the sibling plugin.yaml — the single source of truth."""
    plugin_yaml = pathlib.Path(__file__).parent / "plugin.yaml"

    try:
        for line in plugin_yaml.read_text(encoding="utf-8").splitlines():
            stripped = line.strip()

            if stripped.startswith("version:"):
                return stripped.split(":", 1)[1].strip().strip("\"'")
    except OSError:
        pass

    return "0"


_MCP_PROTOCOL_VERSION = "2025-06-18"
_CLIENT_INFO = {"name": "remindb-hermes-plugin", "version": _read_plugin_version()}
_INSTALL_URL_UNIX = "https://raw.githubusercontent.com/radimsem/remindb/main/install.sh"
_INSTALL_URL_WINDOWS = (
    "https://raw.githubusercontent.com/radimsem/remindb/main/install.ps1"
)


class _MCPStdioClient:
    """Minimal sync MCP-over-stdio client for talking to `remindb serve`."""

    def __init__(self, cmd: List[str], env: Dict[str, str]) -> None:
        self._cmd = cmd
        self._env = env
        self._proc: Optional[subprocess.Popen] = None
        self._next_id = 0
        self._lock = threading.Lock()

    def start(self) -> None:
        self._proc = subprocess.Popen(
            self._cmd,
            env=self._env,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            encoding="utf-8",
            bufsize=1,
            start_new_session=False,
        )
        self.call(
            "initialize",
            {
                "protocolVersion": _MCP_PROTOCOL_VERSION,
                "capabilities": {},
                "clientInfo": _CLIENT_INFO,
            },
        )
        self._send(
            {
                "jsonrpc": "2.0",
                "method": "notifications/initialized",
                "params": {},
            }
        )

    def call(self, method: str, params: Dict[str, Any]) -> Dict[str, Any]:
        with self._lock:
            self._next_id += 1
            req_id = self._next_id

            self._send(
                {
                    "jsonrpc": "2.0",
                    "id": req_id,
                    "method": method,
                    "params": params,
                }
            )

            while True:
                line = self._readline()
                if not line:
                    raise RuntimeError(
                        f"remindb serve closed stdout while awaiting {method}"
                    )

                msg = json.loads(line)
                if msg.get("id") != req_id:
                    logger.debug(
                        "dropping out-of-band MCP message: %s",
                        msg.get("method") or msg.get("id"),
                    )
                    continue

                if "error" in msg:
                    err = msg["error"]
                    raise RuntimeError(
                        f"MCP {method} failed: {err.get('message', err)}"
                    )
                return msg.get("result", {})

    def close(self) -> None:
        proc = self._proc
        self._proc = None
        if proc is None:
            return

        try:
            if proc.stdin:
                proc.stdin.close()
            proc.wait(timeout=2)
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.wait()

    def _send(self, msg: Dict[str, Any]) -> None:
        if not self._proc or not self._proc.stdin:
            raise RuntimeError("MCP stdio client not started")

        self._proc.stdin.write(json.dumps(msg) + "\n")
        self._proc.stdin.flush()

    def _readline(self) -> str:
        if not self._proc or not self._proc.stdout:
            raise RuntimeError("MCP stdio client not started")
        return self._proc.stdout.readline()


def _translate_tool_schema(mcp_tool: Dict[str, Any]) -> Dict[str, Any]:
    """MCP `tools/list` entry → Hermes OpenAI-function-calling schema."""
    return {
        "name": mcp_tool["name"],
        "description": mcp_tool.get("description", ""),
        "parameters": mcp_tool.get("inputSchema")
        or {"type": "object", "properties": {}},
    }


def _binary_available() -> bool:
    if not shutil.which("remindb"):
        return False

    try:
        proc = subprocess.run(
            ["remindb", "--version"],
            capture_output=True,
            timeout=5,
            check=False,
        )
    except (subprocess.TimeoutExpired, OSError):
        return False

    return proc.returncode == 0


class RemindbProvider(MemoryProvider):
    """remindb memory provider — spawns `remindb serve` and proxies MCP."""

    def __init__(self) -> None:
        self._client: Optional[_MCPStdioClient] = None
        self._hermes_home = ""

    @property
    def name(self) -> str:
        return "remindb"

    def is_available(self) -> bool:
        return _binary_available()

    def initialize(self, session_id: str, **kwargs) -> None:
        _ = session_id
        self._hermes_home = kwargs.get("hermes_home", "") or self._hermes_home
        # Connect eagerly so a misconfiguration surfaces in Hermes' init log.
        # Hermes guards this call, so a raise here marks the provider inactive
        # rather than crashing the agent.
        self._ensure_client()

    def get_tool_schemas(self) -> List[Dict[str, Any]]:
        # Hermes calls this at registration — before initialize() — to build
        # its tool-routing table, so the subprocess has to start here too.
        # Never raise: Hermes does not guard get_tool_schemas, and an exception
        # would abort agent startup.
        try:
            client = self._ensure_client()
        except Exception as exc:
            logger.warning("remindb: cannot list tools: %s", exc)
            return []

        result = client.call("tools/list", {})
        return [_translate_tool_schema(t) for t in result.get("tools", [])]

    def handle_tool_call(self, tool_name: str, args: Dict[str, Any], **kwargs) -> str:
        _ = kwargs

        try:
            client = self._ensure_client()
            result = client.call(
                "tools/call",
                {
                    "name": tool_name,
                    "arguments": args,
                },
            )
        except Exception as exc:
            return json.dumps({"error": str(exc)})

        content = result.get("content") or []
        text = next(
            (block.get("text", "") for block in content if block.get("type") == "text"),
            "",
        )
        if result.get("isError"):
            return json.dumps({"error": text or "unknown tool error"})

        return json.dumps({"text": text})

    def shutdown(self) -> None:
        client = self._client
        self._client = None
        if client is not None:
            client.close()

    def get_config_schema(self) -> List[Dict[str, Any]]:
        return [
            {
                "key": "REMINDB_DB",
                "description": "Absolute path to the compiled remindb SQLite database (output of `remindb compile`)",
                "required": True,
            },
            {
                "key": "REMINDB_SOURCE",
                "description": "Absolute path to the workspace directory that compiled into REMINDB_DB",
                "required": True,
            },
        ]

    def save_config(self, values: Dict[str, Any], hermes_home: str) -> None:
        if not hermes_home:
            return

        config_path = os.path.join(hermes_home, "remindb.json")
        existing = self._load_config(hermes_home)
        existing.update(values)

        with open(config_path, "w", encoding="utf-8") as f:
            json.dump(existing, f, indent=2)
            f.write("\n")

    def _ensure_client(self) -> _MCPStdioClient:
        if self._client is not None:
            return self._client

        if not _binary_available():
            install_url = (
                _INSTALL_URL_WINDOWS if sys.platform == "win32" else _INSTALL_URL_UNIX
            )
            raise RuntimeError(
                f"remindb binary not found on PATH. Install with:\n  {install_url}"
            )

        config = self._load_config(self._resolve_hermes_home())
        db = os.environ.get("REMINDB_DB") or config.get("REMINDB_DB", "")
        source = os.environ.get("REMINDB_SOURCE") or config.get("REMINDB_SOURCE", "")

        if not db or not source:
            raise RuntimeError(
                "remindb provider requires REMINDB_DB and REMINDB_SOURCE — "
                "run `hermes memory setup` or export the env vars"
            )

        env = os.environ.copy()
        env["REMINDB_DB"] = db
        env["REMINDB_SOURCE"] = source

        client = _MCPStdioClient(cmd=["remindb", "serve"], env=env)
        client.start()
        self._client = client

        return client

    def _resolve_hermes_home(self) -> str:
        return (
            self._hermes_home
            or os.environ.get("HERMES_HOME")
            or os.path.expanduser("~/.hermes")
        )

    def _load_config(self, hermes_home: str) -> Dict[str, Any]:
        if not hermes_home:
            return {}

        config_path = os.path.join(hermes_home, "remindb.json")
        if not os.path.exists(config_path):
            return {}

        try:
            with open(config_path, encoding="utf-8") as f:
                return json.load(f)
        except (OSError, json.JSONDecodeError):
            return {}


def register(ctx) -> None:
    """Hermes plugin entry point."""
    ctx.register_memory_provider(RemindbProvider())
