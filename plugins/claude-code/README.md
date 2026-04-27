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
printf '%s\n' \
    '# Compile only per-project memory/ markdown; skip the surrounding telemetry.' \
    '' \
    '# Session logs (large, low value).' \
    '*.jsonl' \
    '# Per-session subagent traces (any depth).' \
    'subagents/' \
    '# Per-session tool outputs (any depth).' \
    'tool-results/' \
    > ~/.claude/projects/.remindb.ignore
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

### 4. Export the workspace env vars

`remindb serve` reads `REMINDB_DB` and `REMINDB_SOURCE` as fallbacks for its `--db` and `--source` flags. The plugin's bundled `.mcp.json` declares both as `${VAR}` passthroughs into the spawned subprocess, so export them in the shell that launches Claude Code:

```bash
export REMINDB_DB=$HOME/.cache/remindb/claude.db
export REMINDB_SOURCE=$HOME/.claude/projects
```

Put them in `~/.bashrc` / `~/.zshrc` / fish equivalent to make the mapping permanent, or scope them to a single session to switch workspaces between runs. Undefined `${VAR}` references in the bundled `.mcp.json` resolve to empty strings, and `remindb` then falls back to a relative `memory.db` in Claude Code's cwd — so set both before launching, not after.

A same-named server in user-scope `~/.claude.json` *replaces* the plugin's bundled entry per Claude Code's MCP precedence rules (it does not merge), so don't try to inject env there.

## Configuration

The plugin itself has no runtime options. `remindb serve` resolves its DB and source paths from `REMINDB_DB` and `REMINDB_SOURCE` at launch.

## Tools exposed

The plugin surfaces the full `remindb` `Memory*` tool suite under the `remindb__` namespace. See the [main README](https://github.com/radimsem/remindb#mcp-tools) for the canonical tool list and token-savings benchmarks per tool.

## License

MIT — same as remindb.
