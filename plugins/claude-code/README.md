# remindb Plugin for Claude Code

Mounts the [remindb](https://github.com/radimsem/remindb) MCP server in Claude Code, exposing a compiled SQLite view of your workspace as eight token-budgeted tools:
`MemoryTree`, `MemorySearch`, `MemoryFetch`, `MemoryWrite`, `MemoryDelta`, `MemorySummarize`, `MemoryHistory`, `MemoryCompile`.

## How it works

Claude Code loads `.claude-plugin/plugin.json` as the plugin manifest and merges `.mcp.json` into its effective MCP server list. When Claude Code starts with the plugin enabled, it spawns `remindb serve` over stdio. All tool logic lives in the Go binary; the plugin is a thin wrapper.

Tools are namespaced, so `MemoryFetch` appears as `remindb__MemoryFetch` in the tool list.

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

remindb needs a SQLite file populated from a source tree before the agent can read from it. A natural source for Claude Code is its own memory folder at `~/.claude/` — `CLAUDE.md` files, per-project auto memory under `~/.claude/projects/<project>/memory/`, and user-level rules. Indexing it lets Claude query its own persistent context through remindb instead of grepping the dot folder:

```bash
mkdir -p ~/.cache/remindb
remindb compile ~/.claude --db ~/.cache/remindb/claude.db
```

Or point at any other workspace you want agents to see — a docs tree, a notes repo, a project directory. Re-run whenever you want a fresh baseline; `serve` keeps the DB current after that.

### 3. Install the plugin

Pick one of:

**From the GitHub repo via marketplace** (recommended — users installing from a raw GitHub URL):

```
/plugin marketplace add radimsem/remindb
/plugin install remindb@remindb
```

**Local checkout** (for plugin development):

```bash
claude --plugin-dir ./plugins/claude-code
```

After either path, confirm the server is connected:

```
/mcp
```

You should see `remindb` listed with eight `MemoryXxx` tools.

### 4. Point remindb at your workspace via `~/.claude/mcp.json`

`remindb serve` reads `REMINDB_DB` and `REMINDB_SOURCE` as fallbacks for its `--db` and `--source` flags. The cleanest place to set them for Claude Code is the user-level MCP override file at `~/.claude/mcp.json`, which Claude Code merges on top of every plugin's bundled `.mcp.json` without mutating the plugin itself:

```json
{
    "mcpServers": {
        "remindb": {
            "env": {
                "REMINDB_DB": "${HOME}/.cache/remindb/claude.db",
                "REMINDB_SOURCE": "${HOME}/.claude"
            }
        }
    }
}
```

Claude Code expands `${VAR}` (with braces) in `.mcp.json` values — bare `$HOME` is treated as a literal string and won't work.

This scopes the env vars to Claude Code's spawned subprocess, survives `/plugin update remindb`, and lets you switch sources by editing one file and running `/reload-plugins`.

Prefer a shell-inherited env instead? Export the same pair in `~/.bashrc` / `~/.zshrc` / fish equivalent and restart Claude Code from that shell.

## Configuration

The plugin itself has no runtime options. `remindb serve` resolves its DB and source paths from `REMINDB_DB` and `REMINDB_SOURCE` at launch; explicit `--db` / `--source` flags in `~/.claude/mcp.json` override the env vars if you need per-bundle pinning.

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
