# remindb Plugin for Claude Code

Mounts the [remindb](https://github.com/radimsem/remindb) MCP server in Claude Code, exposing a compiled SQLite view of your workspace as the full `Memory*` tool suite.

## How it works

Claude Code loads `.claude-plugin/plugin.json` as the plugin manifest and merges `.mcp.json` into its effective MCP server list. When Claude Code starts with the plugin enabled, it spawns `remindb serve` over stdio. All tool logic lives in the Go binary; the plugin is a thin wrapper.

Tools are namespaced, so `MemoryFetch` appears as `remindb__MemoryFetch` in the tool list.

## Installation

### 1. Install the remindb binary

The binary must be on `$PATH`:

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

remindb needs a SQLite file populated from a source tree before the agent can read from it. A natural source for Claude Code is its per-project auto-memory at `~/.claude/projects/<project>/memory/` — markdown files Claude has accumulated about each repo it works in. Indexing them across all projects lets Claude query its own persistent memory through remindb instead of grepping the dot folder.

`~/.claude/projects/<project>/` sits next to several other artifacts that don't belong in long-term memory: session-log `.jsonl` files, plus `subagents/` and `tool-results/` subtrees under each session UUID directory. Drop a `.remindb.ignore` at `~/.claude/projects/` to filter them out — so the only thing the compiler ingests is `memory/*.md` per project:

```bash
mkdir -p ~/.cache/remindb
cat > ~/.claude/projects/.remindb.ignore <<'EOF'
# Compile only per-project memory/ markdown; skip the surrounding telemetry.
*.jsonl              # session logs (large, low value)
subagents/           # per-session subagent traces (any depth)
tool-results/        # per-session tool outputs (any depth)
EOF
remindb compile ~/.claude/projects --db ~/.cache/remindb/claude.db
```

The same `.remindb.ignore` is honored by `serve`'s background rescan and the `MemoryCompile` MCP tool — set it once, all paths agree. If Claude Code adds a new sibling-of-`memory/` artifact in a future release, append it to the file and recompile. Or point at any other workspace you want agents to see — a docs tree, a notes repo, a project directory. Re-run whenever you want a fresh baseline; `serve` keeps the DB current after that.

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

You should see `remindb` listed with the full `Memory*` tool suite.

### 4. Point remindb at your workspace via `~/.claude/mcp.json`

`remindb serve` reads `REMINDB_DB` and `REMINDB_SOURCE` as fallbacks for its `--db` and `--source` flags. The cleanest place to set them for Claude Code is the user-level MCP override file at `~/.claude/mcp.json`, which Claude Code merges on top of every plugin's bundled `.mcp.json` without mutating the plugin itself:

```json
{
    "mcpServers": {
        "remindb": {
            "env": {
                "REMINDB_DB": "${HOME}/.cache/remindb/claude.db",
                "REMINDB_SOURCE": "${HOME}/.claude/projects"
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

The plugin surfaces the full `remindb` `Memory*` tool suite under the `remindb__` namespace. See the [main README](https://github.com/radimsem/remindb#mcp-tools) for the canonical tool list and token-savings benchmarks per tool.

## License

MIT — same as remindb.
