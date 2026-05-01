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

### 2. Compile your workspace

remindb needs a SQLite file built from a source tree before the agent can read from it. The source is whatever workspace you want OpenCode to remember — a code repo, a docs tree, a notes directory.

```bash
mkdir -p ~/.cache/remindb
remindb compile ~/code/my-project --db ~/.cache/remindb/my-project.db
```

Drop a `.remindb.ignore` at the workspace root if you need to exclude noise (build outputs, vendored deps, generated files). The same file is honored by `serve`'s background rescan and the `MemoryCompile` tool.

#### Bring OpenCode's hierarchical memory along

OpenCode doesn't keep a `memory/` folder — its persistent context is a stack of `AGENTS.md` files. It loads them from three places: the global `~/.config/opencode/AGENTS.md`, project-root and ancestor `AGENTS.md` files traversed upward from your cwd, and a Claude Code fallback at `~/.claude/CLAUDE.md` (unless disabled). Only `AGENTS.md` files at or below the workspace root land in `REMINDB_SOURCE` automatically — ancestors above it and the global file live outside.

Ask OpenCode to compile them once the plugin is running. Use absolute paths — `MemoryCompile` doesn't expand `~`:

```
remindb__MemoryCompile(path="/home/you/.config/opencode/AGENTS.md", message="seed: global memory")
remindb__MemoryCompile(path="/home/you/code/parent/AGENTS.md", message="seed: ancestor memory")
remindb__MemoryCompile(path="/home/you/.claude/CLAUDE.md", message="seed: claude-code fallback")
```

Re-run whenever the file changes.

### 3. Add the MCP entry to your `opencode.json`

Pick one:

**Project-level** (recommended — one workspace per repo):

```bash
curl -fsSL https://raw.githubusercontent.com/radimsem/remindb/main/plugins/opencode/opencode.json \
    -o opencode.json
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
                "REMINDB_DB": "{env:HOME}/.cache/remindb/my-project.db",
                "REMINDB_SOURCE": "{env:HOME}/code/my-project"
            },
            "enabled": true
        }
    }
}
```

Heads up: OpenCode only expands `{env:VARIABLE_NAME}` in config values — shell-style `$HOME` or `${HOME}` is treated as a literal string and won't work. Swap the paths for a different workspace (e.g., `{env:HOME}/notes` + `{env:HOME}/.cache/remindb/notes.db`) whenever you want OpenCode to read a different tree. Keep the file per-project at the workspace root as `opencode.json` so each workspace carries its own DB and source paths — OpenCode reads it on session start, so launching a fresh session from the new directory is enough to swap configs.

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
