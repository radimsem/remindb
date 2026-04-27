# remindb Plugin for Codex

Mounts the [remindb](https://github.com/radimsem/remindb) MCP server as a workspace memory backend for OpenAI Codex agents.

Agents get the full `remindb__Memory*` tool suite — backed by a compiled SQLite view of the workspace.

## How it works

Codex loads `.codex-plugin/plugin.json` as the plugin manifest. The manifest's `mcpServers` field points at `.mcp.json`, which Codex uses to spawn `remindb serve` over stdio.

All tool logic lives in the Go binary; the plugin is a thin wrapper.

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

remindb needs a SQLite file populated from a source tree before the agent can read from it. A natural source for Codex is its own state folder at `~/.codex/` — custom slash-command prompts under `prompts/`, persistent context under `memories/` and `memories_extensions/`, and any user-authored `skills/`. Indexing it lets Codex query its own persistent context through remindb instead of grepping the dot folder.

`~/.codex/` also accumulates session-rollout `.jsonl` files under `sessions/YYYY/MM/DD/`, an `archived_sessions/` subtree, and a top-level `history.jsonl` — large transcripts that bloat the index without adding agent-memory value. Drop a `.remindb.ignore` at `~/.codex/` to filter them out.

```bash
mkdir -p ~/.cache/remindb
printf '%s\n' \
    '# Compile only curated context; skip session rollouts and history.' \
    '' \
    '# history.jsonl + per-day session rollouts.' \
    '*.jsonl' \
    '# Rollout subtree under YYYY/MM/DD.' \
    'sessions/' \
    '# Archived rollout subtree.' \
    'archived_sessions/' \
    > ~/.codex/.remindb.ignore
remindb compile ~/.codex --db ~/.cache/remindb/codex.db
```

The same `.remindb.ignore` is honored by `serve`'s background rescan and the `MemoryCompile` MCP tool — set it once, all paths agree. Or point at any other workspace you want agents to see — a docs tree, a notes repo, a project directory.

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

You should see `remindb` listed with the full `Memory*` tool suite.

### 4. Export the workspace env vars

`remindb serve` reads `REMINDB_DB` and `REMINDB_SOURCE` as fallbacks for its `--db` and `--source` flags. Codex propagates the launching shell's environment to plugin-spawned MCP subprocesses, so export them in the shell that launches Codex:

```bash
export REMINDB_DB=$HOME/.cache/remindb/codex.db
export REMINDB_SOURCE=$HOME/.codex
```

Put them in `~/.bashrc` / `~/.zshrc` / fish equivalent to make the mapping permanent, or scope them to a single session to switch workspaces between runs.

If shell-rc isn't an option, sidestep the plugin entirely and define a top-level `[mcp_servers.remindb]` block in `~/.codex/config.toml` instead — Codex's `[plugins.<name>]` table accepts only `enabled` and performs no `${VAR}` / `$VAR` / `{env:VAR}` expansion in either `config.toml` or the plugin's bundled `.mcp.json`, so there is no first-class way to inject env into a plugin-bundled MCP server from user config:

```toml
[mcp_servers.remindb]
command = "remindb"
args = ["serve"]
env = { REMINDB_DB = "/home/you/.cache/remindb/codex.db", REMINDB_SOURCE = "/home/you/.codex" }
```

Replace `/home/you` with your absolute `$HOME` — `config.toml` does not expand it. This registers `remindb` as a user-defined MCP server, not a plugin server, so the plugin can stay disabled or removed entirely if you take this path.

## Tools exposed

The plugin surfaces the full `remindb` `Memory*` tool suite under the `remindb__` namespace. See the [main README](https://github.com/radimsem/remindb#mcp-tools) for the canonical tool list and token-savings benchmarks per tool.

## License

MIT — same as remindb.
