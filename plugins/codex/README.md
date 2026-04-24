# remindb Plugin for Codex

Mounts the [remindb](https://github.com/radimsem/remindb) MCP server as a workspace memory backend for OpenAI Codex agents.

Agents get eight `remindb__*` tools — `MemoryFetch`, `MemorySearch`, `MemoryWrite`, `MemoryCompile`, `MemoryDelta`, `MemorySummarize`, `MemoryHistory`, `MemoryTree` — backed by a compiled SQLite view of the workspace.

## How it works

Codex loads `.codex-plugin/plugin.json` as the plugin manifest. The manifest's `mcpServers` field points at `.mcp.json`, which Codex uses to spawn `remindb serve` over stdio.

All tool logic lives in the Go binary; the plugin is a thin wrapper.

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

remindb needs a SQLite file populated from a source tree before the agent can read from it. A natural source for Codex is its own state folder at `~/.codex/` — user-level `AGENTS.md` / `AGENTS.override.md`, session transcripts in `history.jsonl`, logs under `log/`, and `config.toml`. Indexing it lets Codex query its own persistent context through remindb instead of grepping the dot folder:

```bash
mkdir -p ~/.cache/remindb
remindb compile ~/.codex --db ~/.cache/remindb/codex.db
```

Or point at any other workspace you want agents to see — a docs tree, a notes repo, a project directory.

### 3. Add the plugin from GitHub

```bash
codex plugin marketplace add radimsem/remindb --sparse plugins/codex
codex plugin install remindb
```

The plugin is cached at `~/.codex/plugins/cache/remindb/remindb/<version>/`. On/off state lives in `~/.codex/config.toml`.

Confirm the server is connected:

```bash
codex mcp list
```

You should see `remindb` listed with eight `MemoryXxx` tools.

### 4. Point remindb at your workspace via `~/.codex/config.toml`

`remindb serve` reads `REMINDB_DB` and `REMINDB_SOURCE` as fallbacks for its `--db` and `--source` flags. The cleanest place to set them for Codex is `~/.codex/config.toml`, which Codex merges into every plugin-launched MCP subprocess without mutating the cached plugin:

```toml
[plugins.remindb.mcpServers.remindb.env]
REMINDB_DB = "/home/you/.cache/remindb/codex.db"
REMINDB_SOURCE = "/home/you/.codex"
```

Replace `/home/you` with your `$HOME`. Codex's `config.toml` has no documented env-var expansion, so shell-style `$HOME` is treated as a literal string — use absolute paths here, or drop this block and rely on the shell-inherited env fallback below.

This scopes the env vars to Codex's spawned subprocess, survives `codex plugin update remindb`, and lets you switch sources by editing one file and restarting Codex.

Prefer a shell-inherited env instead? Export the same pair in `~/.bashrc` / `~/.zshrc` / fish equivalent and restart Codex from that shell.

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
