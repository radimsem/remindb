#!/usr/bin/env python3
"""Smoke-test the Hermes Agent memory provider against an isolated
HERMES_HOME so the user's real ~/.hermes is never touched.

Scope: this is a REGRESSION GUARD for issue #169 (defining `post_setup`
made the wizard short-circuit) plus an activation smoke -- NOT a full
behavioural test of the wizard's interactive schema-prompt loop, which
runs under curses and can only be driven with a pty. The bypass below
uses `cmd_setup_provider` (named entrypoint) instead of `cmd_setup`
(picker entrypoint); both short-circuit on `hasattr(post_setup)` the
same way, so the regression guard is faithful, but the schema-prompt
iteration at memory_setup.py:280-340 is only checked structurally
(field shape matches what the loop expects), not exercised at runtime.

Asserts:
  1. `hermes` and `remindb` are on PATH
  2. Memory provider discovery finds remindb after copy-install
  3. RemindbProvider does NOT define `post_setup` (the #169 guard)
  4. `get_config_schema()` exposes both required fields in a shape
     the wizard's prompt loop can iterate (no `when`/`default_from`,
     non-empty descriptions)
  5. `provider.save_config()` writes ~/.hermes/remindb.json
  6. `cmd_setup_provider` activates `memory.provider` in config.yaml
  7. `hermes memory status` reports `Provider: remindb`

Usage: python3 plugins/hermes-agent/memory/remindb/test-smoke.py
"""

from __future__ import annotations

import importlib.util
import json
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import NoReturn

PLUGIN_DIR = Path(__file__).resolve().parent


def fail(msg: str) -> NoReturn:
    sys.exit(f"FAIL: {msg}")


def check_cli(name: str) -> str:
    path = shutil.which(name)
    if not path:
        fail(f"{name} not on PATH")

    proc = subprocess.run(
        [path, "--version"], capture_output=True, text=True, check=False
    )
    if proc.returncode != 0:
        return f"{name} (version probe failed)"

    return (proc.stdout or proc.stderr).splitlines()[0]


def ensure_hermes_importable() -> None:
    if importlib.util.find_spec("hermes_cli.memory_setup") is not None:
        return

    fallback = Path(
        os.environ.get(
            "HERMES_AGENT_DIR", str(Path.home() / ".hermes" / "hermes-agent")
        )
    )
    if not fallback.is_dir():
        fail(f"hermes_cli not importable and HERMES_AGENT_DIR not found: {fallback}")

    sys.path.insert(0, str(fallback))


def main() -> None:
    print(f"hermes:  {check_cli('hermes')}")
    print(f"remindb: {check_cli('remindb')}")

    with tempfile.TemporaryDirectory(prefix="hermes-smoke.") as tmp:
        home = Path(tmp)
        os.environ["HERMES_HOME"] = str(home)
        (home / "plugins").mkdir()
        shutil.copytree(PLUGIN_DIR, home / "plugins" / "remindb")

        (home / "seed.md").write_text("# smoke\n")
        db = home / "smoke.db"
        subprocess.run(
            ["remindb", "compile", str(home), "--db", str(db)],
            check=True,
            capture_output=True,
        )

        ensure_hermes_importable()

        from plugins.memory import discover_memory_providers, load_memory_provider
        from hermes_cli.config import load_config
        from hermes_cli.memory_setup import cmd_setup_provider

        discovered = {name for name, _, _ in discover_memory_providers()}
        assert "remindb" in discovered, (
            f"discovery missed remindb; found: {sorted(discovered)}"
        )

        provider = load_memory_provider("remindb")
        assert provider is not None, "load_memory_provider returned None"
        assert provider.name == "remindb"

        assert not hasattr(provider, "post_setup"), (
            "REGRESSION (issue #169): RemindbProvider defines post_setup; the Hermes wizard will short-circuit"
        )

        schema = provider.get_config_schema()
        schema_keys = {f["key"] for f in schema}
        assert schema_keys == {"REMINDB_DB", "REMINDB_SOURCE"}, (
            f"schema keys: {schema_keys}"
        )
        assert all(f.get("required") for f in schema), "fields must be required"

        # The wizard's prompt loop (memory_setup.py:280-340) iterates the schema
        # and branches on the keys below. Our fields must be plain-text required
        # prompts for the loop to ask for both paths.
        forbidden = ("when", "default_from", "choices", "secret")
        for f in schema:
            assert f.get("description"), (
                f"field {f['key']!r} needs a non-empty description"
            )

            for key in forbidden:
                assert key not in f, (
                    f"field {f['key']!r} sets {key!r}; wizard would branch and skip it"
                )

        provider.save_config(
            {"REMINDB_DB": str(db), "REMINDB_SOURCE": str(home)}, str(home)
        )

        remindb_json = json.loads((home / "remindb.json").read_text())
        expected = {"REMINDB_DB": str(db), "REMINDB_SOURCE": str(home)}
        assert remindb_json == expected, f"remindb.json mismatch: {remindb_json}"

        cmd_setup_provider("remindb")

        cfg = load_config()
        provider_key = cfg.get("memory", {}).get("provider")
        assert provider_key == "remindb", (
            f"memory.provider not set; got {provider_key!r}"
        )

        print("python checks passed")

        status = subprocess.run(
            ["hermes", "memory", "status"],
            capture_output=True,
            text=True,
            check=False,
        )
        combined = status.stdout + status.stderr
        if "Provider:" not in combined or "remindb" not in combined:
            print(combined)
            fail("'hermes memory status' does not report Provider: remindb")

        print(f"OK: Hermes memory provider smoke passed (HERMES_HOME={home})")


if __name__ == "__main__":
    main()
