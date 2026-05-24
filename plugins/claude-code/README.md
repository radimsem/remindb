# remindb for Claude Code

Drops [remindb](https://github.com/radimsem/remindb) into Claude Code as an MCP server. The agent picks up the full `Memory*` tool suite, backed by a compiled SQLite view of whatever workspace you point it at.

## How it works

Claude Code loads `.claude-plugin/plugin.json` as the plugin manifest and merges `.mcp.json` into its effective MCP server list. When Claude Code starts with the plugin enabled, it spawns `remindb serve` over stdio. All the tool logic lives in the Go binary; the plugin is a thin wrapper.

Tools are namespaced, so `MemoryFetch` shows up as `remindb__MemoryFetch` in the tool list.

## Installation

Setup is **config-first and wizard-driven**: install the binary and skills, run `/remindb-setup` *before* the plugin is attached (it authors `.remindb/`, compiles, and wires the env for you), then enable the plugin and restart. You don't hand-compile or hand-edit config — the wizard owns that.

### 1. Install the remindb binary

It needs to be on `$PATH`:

```bash
curl -fsSL https://raw.githubusercontent.com/radimsem/remindb/main/install.sh | bash
```

On Windows:

```powershell
iwr -useb https://raw.githubusercontent.com/radimsem/remindb/main/install.ps1 | iex
```

Verify: `remindb --version`.

### 2. Install the companion skills

This pulls the `remindb-setup` wizard plus the `remind` (read) and `memorize` (write) skills:

```bash
npx skills@latest add radimsem/remindb/skills -a claude-code
```

Refresh them later — independent of `remindb update` — with `npx skills@latest update`.

### 3. Run the setup wizard

In a Claude Code session, invoke the wizard as a slash command:

```
/remindb-setup            # interactive — walks each choice
/remindb-setup automode   # hands-off — infers the whole setup
```

Running before the plugin is attached is intended: the wizard detects the host, authors `.remindb/` (`ignore`/`pinned`/`temperatures.json`/`config.json`) **before** compiling, runs `remindb compile`, offers to seed adjacent context (this project's `CLAUDE.md`, in-repo `README.md`), and wires the MCP env. A natural source for Claude Code is its own cross-project memory at `~/.claude/projects/` — the wizard will suggest it.

### 4. Enable the plugin

```
/plugin marketplace add radimsem/remindb
/plugin install remindb@remindb
```

Or, if you're hacking on the plugin, a local checkout: `claude --plugin-dir ./plugins/claude-code`.

### 5. Restart and verify

Start a new session (so the freshly-wired env and plugin take effect), then confirm the server is connected:

```
/mcp
```

You should see `remindb` with the full `Memory*` tool suite. Re-run `/remindb-setup` — now that the server is attached, it runs its verify pass (`MemoryStats` + `remindb://doctor`).

## Durable env

`remindb serve` reads `REMINDB_DB` and `REMINDB_SOURCE` as fallbacks for its `--db`/`--source` flags, and the bundled `.mcp.json` declares both as `${VAR}` passthroughs. For a **marketplace install** the manifest lives in the plugin cache under `~/.claude/plugins/` and is overwritten on every update, so the wizard can't durably write it — it emits a shell-export snippet instead, which you add to `~/.bashrc` / `~/.zshrc` / your fish config:

```bash
export REMINDB_DB=$HOME/.cache/remindb/claude.db
export REMINDB_SOURCE=$HOME/.claude/projects
```

Launch Claude Code from that shell. For a **local checkout** (`--plugin-dir`) you own the file, so the wizard writes the paths straight into the `env` block of `.mcp.json`. Either way, don't inject a same-named server into user-scope `~/.claude.json` — Claude Code *replaces* the plugin's bundled entry rather than merging it, dropping the plugin wiring entirely.

## Tools exposed

The plugin surfaces the full `remindb` `Memory*` tool suite under the `remindb__` namespace. See the [main README](https://github.com/radimsem/remindb#mcp-tools) for the canonical tool list and per-tool token-savings benchmarks.

## License

MIT — same as remindb.
