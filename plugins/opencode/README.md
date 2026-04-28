# remindb for OpenCode

Drops [remindb](https://github.com/radimsem/remindb) into OpenCode as an MCP server. The agent picks up the full `remindb__Memory*` tool suite, backed by a compiled SQLite view of whatever workspace you point it at.

## How it works

OpenCode configures MCP servers in `opencode.json` under the top-level `mcp` object rather than via the plugin API. This folder ships:

- `opencode.json` — a ready-to-merge MCP entry that spawns `remindb serve` over stdio.
- `plugin.ts` — a minimal OpenCode plugin stub so the bundle can be distributed as an npm package for users who prefer that path.

## Installation

### 1. Install the remindb binary

It needs to be on `$PATH`:

```bash
curl -fsSL https://raw.githubusercontent.com/radimsem/remindb/main/install.sh | bash
```

On Windows:

```powershell
iwr -useb https://raw.githubusercontent.com/radimsem/remindb/main/install.ps1 | iex
```

Verify:

```bash
remindb --version
```

### 2. Compile a source directory

remindb needs a SQLite file built from a source tree before the agent can read from it.

A natural source for OpenCode is its own config folder at `~/.config/opencode/` — user-level `AGENTS.md`, `agents/` definitions, `commands/` templates, `plugins/`, `themes/`, and `opencode.json`. Indexing it lets OpenCode query its own persistent context through remindb instead of grepping the dot folder:

```bash
mkdir -p ~/.cache/remindb
remindb compile ~/.config/opencode --db ~/.cache/remindb/opencode.db
```

Or point at any other workspace you want the agent to see — a docs tree, a notes repo, a project directory.

### 3. Add the MCP entry to your `opencode.json`

Pick one:

**Project-level** (recommended — one workspace per repo):

```bash
curl -fsSL https://raw.githubusercontent.com/radimsem/remindb/main/plugins/opencode/opencode.json \
    -o .opencode/opencode.json
```

**Global** (applies to every OpenCode session):

```bash
mkdir -p ~/.config/opencode
curl -fsSL https://raw.githubusercontent.com/radimsem/remindb/main/plugins/opencode/opencode.json \
    -o ~/.config/opencode/opencode.json
```

Or merge this block into an existing config by hand:

```json
{
    "$schema": "https://opencode.ai/config.json",
    "mcp": {
        "remindb": {
            "type": "local",
            "command": ["remindb", "serve"],
            "enabled": true
        }
    }
}
```

**Optional — npm-distributed plugin stub.** If you want the bundle to show up in OpenCode's plugin list, reference the npm package from the same `opencode.json`:

```json
{
    "plugin": ["@radimsem/remindb-opencode"]
}
```

OpenCode runs `bun install` at startup to resolve the dependency.

Confirm the server is connected:

```bash
opencode mcp list
```

You should see `remindb` listed with the full `Memory*` tool suite.

### 4. Point remindb at your workspace via `opencode.json`

`remindb serve` reads `REMINDB_DB` and `REMINDB_SOURCE` as fallbacks for its `--db` and `--source` flags. The cleanest place to set them for OpenCode is the `environment` object on the same `mcp.remindb` entry — OpenCode passes it straight to the spawned subprocess without touching your shell env:

```json
{
    "$schema": "https://opencode.ai/config.json",
    "mcp": {
        "remindb": {
            "type": "local",
            "command": ["remindb", "serve"],
            "environment": {
                "REMINDB_DB": "{env:HOME}/.cache/remindb/opencode.db",
                "REMINDB_SOURCE": "{env:HOME}/.config/opencode"
            },
            "enabled": true
        }
    }
}
```

Heads up: OpenCode only expands `{env:VARIABLE_NAME}` in config values — shell-style `$HOME` or `${HOME}` is treated as a literal string and won't work. Swap the paths for a different workspace (e.g., `{env:HOME}/notes` + `{env:HOME}/.cache/remindb/notes.db`) whenever you want OpenCode to read a different tree. Keep the file per-project under `.opencode/opencode.json` so each workspace carries its own DB and source paths — no restart needed when you switch repos, just `opencode mcp restart remindb`.

Prefer a shell-inherited env? Point the two values at your own env vars via the same substitution:

```json
"environment": {
    "REMINDB_DB": "{env:REMINDB_DB}",
    "REMINDB_SOURCE": "{env:REMINDB_SOURCE}"
}
```

Then export the pair in `~/.bashrc` / `~/.zshrc` / your fish equivalent and restart OpenCode from that shell.

## Tools exposed

The plugin surfaces the full `remindb` `Memory*` tool suite under the `remindb__` namespace. See the [main README](https://github.com/radimsem/remindb#mcp-tools) for the canonical tool list and per-tool token-savings benchmarks.

## License

MIT — same as remindb.
