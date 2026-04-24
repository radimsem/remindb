# remindb Plugin for OpenCode

Mounts the [remindb](https://github.com/radimsem/remindb) MCP server as a workspace memory backend for OpenCode agents.

Agents get eight `remindb__*` tools — `MemoryFetch`, `MemorySearch`, `MemoryWrite`, `MemoryCompile`, `MemoryDelta`, `MemorySummarize`, `MemoryHistory`, `MemoryTree` — backed by a compiled SQLite view of the workspace.

## How it works

OpenCode configures MCP servers in `opencode.json` under the top-level `mcp` object rather than via the plugin API. This folder ships:

- `opencode.json` — a ready-to-merge MCP entry that spawns `remindb serve` over stdio.
- `plugin.ts` — a minimal OpenCode plugin stub so the bundle can be distributed as an npm package for users who prefer that path.

## Installation

### 1. Install the remindb binary

The binary must be on `$PATH`:

```bash
curl -fsSL https://raw.githubusercontent.com/radimsem/remindb/master/install.sh | sh
```

On Windows:

```powershell
iwr -useb https://raw.githubusercontent.com/radimsem/remindb/master/install.ps1 | iex
```

Verify:

```bash
remindb --help
```

### 2. Compile a source directory

remindb needs a SQLite file populated from a source tree before the agent can read from it. A natural source for OpenCode is its own config folder at `~/.config/opencode/` — user-level `AGENTS.md`, `agents/` definitions, `commands/` templates, `plugins/`, `themes/`, and `opencode.json`. Indexing it lets OpenCode query its own persistent context through remindb instead of grepping the dot folder:

```bash
mkdir -p ~/.cache/remindb
remindb compile ~/.config/opencode --db ~/.cache/remindb/opencode.db
```

Or point at any other workspace you want agents to see — a docs tree, a notes repo, a project directory.

### 3. Add the MCP entry to your `opencode.json`

Pick one of:

**Project-level** (recommended — one workspace per repo):

```bash
curl -fsSL https://raw.githubusercontent.com/radimsem/remindb/master/opencode/opencode.json \
    -o .opencode/opencode.json
```

**Global** (applies to every OpenCode session):

```bash
mkdir -p ~/.config/opencode
curl -fsSL https://raw.githubusercontent.com/radimsem/remindb/master/opencode/opencode.json \
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

**Optional — npm-distributed plugin stub.** If you want the bundle to appear in OpenCode's plugin list, reference the npm package from the same `opencode.json`:

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

You should see `remindb` listed with eight `MemoryXxx` tools.

### 4. Point remindb at your workspace via `opencode.json`

`remindb serve` reads `REMINDB_DB` and `REMINDB_SOURCE` as fallbacks for its `--db` and `--source` flags. The cleanest place to set them for OpenCode is the `environment` object on the same `mcp.remindb` entry — OpenCode passes it straight to the spawned subprocess without mutating shell env:

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

OpenCode only expands `{env:VARIABLE_NAME}` in config values — shell-style `$HOME` or `${HOME}` is treated as a literal string and won't work. Swap the paths for a different workspace (e.g., `{env:HOME}/notes` + `{env:HOME}/.cache/remindb/notes.db`) whenever you want OpenCode to read a different tree. Keep the file per-project under `.opencode/opencode.json` so each workspace carries its own DB and source paths — no restarts needed when you switch repos, just `opencode mcp restart remindb`.

Prefer a shell-inherited env instead? Point the two values at your own env vars via the same substitution:

```json
"environment": {
    "REMINDB_DB": "{env:REMINDB_DB}",
    "REMINDB_SOURCE": "{env:REMINDB_SOURCE}"
}
```

Then export the pair in `~/.bashrc` / `~/.zshrc` / fish equivalent and restart OpenCode from that shell.

## Tools exposed

| Tool | Purpose |
|------|---------|
| `remindb__MemoryTree` | Structural overview of the compiled workspace |
| `remindb__MemorySearch` | Full-text search within a token budget |
| `remindb__MemoryFetch` | Context around an anchor node within a token budget |
| `remindb__MemoryWrite` | Write or update content at an anchor, creating a snapshot |
| `remindb__MemoryDelta` | Changes since a given snapshot |
| `remindb__MemorySummarize` | Replace a node's content with a provided summary |
| `remindb__MemoryHistory` | Browse version history for a node |
| `remindb__MemoryCompile` | Re-compile source files or a directory |

See the [remindb README](https://github.com/radimsem/remindb#readme) for token-savings benchmarks per tool.

## License

MIT — same as remindb.
