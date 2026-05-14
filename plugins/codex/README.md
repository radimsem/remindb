# remindb for Codex

Drops [remindb](https://github.com/radimsem/remindb) into OpenAI Codex as an MCP server. The agent picks up the full `remindb__Memory*` tool suite, backed by a compiled SQLite view of whatever workspace you point it at.

## How it works

Codex treats the repository as a marketplace catalog (`.agents/plugins/marketplace.json` at the repo root) that lists one plugin, `plugins/codex/remindb/`. The plugin's `.codex-plugin/plugin.json` points at sibling `.mcp.json`, which Codex uses to spawn `remindb serve` over stdio.

All tool logic lives in the Go binary; the plugin is a thin wrapper.

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

A natural source for Codex is its own persistent context at `~/.codex/memories/` — markdown files Codex accumulates as long-term memory across sessions. Indexing them lets Codex query its own memory through remindb instead of grepping the dot folder.

```bash
mkdir -p ~/.cache/remindb
remindb compile ~/.codex/memories --db ~/.cache/remindb/codex.db
```

`memories/` is pure user content, so no `.remindb.ignore` is needed. Skills under `~/.codex/skills/` and slash-command prompts under `~/.codex/prompts/` are deliberately *not* indexed — Codex already loads them as live instructions, so re-indexing them in remindb would double-count. Or point at any other workspace you want the agent to see — a docs tree, a notes repo, a project directory.

### 3. Point remindb at your workspace

`remindb serve` reads `REMINDB_DB` and `REMINDB_SOURCE` as fallbacks for its `--db` and `--source` flags. Codex propagates the launching shell's environment to plugin-spawned MCP subprocesses, so export them in the shell **before launching Codex with the plugin enabled** — otherwise the first activation falls back to a stray `memory.db` in cwd:

```bash
export REMINDB_DB=$HOME/.cache/remindb/codex.db
export REMINDB_SOURCE=$HOME/.codex/memories
```

Stick them in `~/.bashrc` / `~/.zshrc` / your fish equivalent to make it permanent, or scope to a single session if you want to switch workspaces between runs.

If shell-rc isn't an option for you, sidestep the plugin entirely and define a top-level `[mcp_servers.remindb]` block in `~/.codex/config.toml` instead.

Why the workaround? Codex's `[plugins.<name>]` table only accepts `enabled` and does no `${VAR}` / `$VAR` / `{env:VAR}` expansion in either `config.toml` or the plugin's bundled `.mcp.json` (which is why the bundled `.mcp.json` ships with `env: {}` — placeholders would be passed through literally and override the inherited shell values with garbage). There's no first-class way to inject env into a plugin-bundled MCP server from user config. So:

```toml
[mcp_servers.remindb]
command = "remindb"
args = ["serve"]
env = { REMINDB_DB = "/home/you/.cache/remindb/codex.db", REMINDB_SOURCE = "/home/you/.codex/memories" }
```

Replace `/home/you` with your absolute `$HOME` — `config.toml` does not expand it. This registers `remindb` as a user-defined MCP server, not a plugin server, so the plugin can stay disabled or removed entirely if you take this path.

### 4. Add the plugin from GitHub

```bash
codex plugin marketplace add radimsem/remindb
```

That single command does both jobs: the marketplace's `policy.installation: INSTALLED_BY_DEFAULT` makes Codex install the bundled plugin in the same step. The plugin caches at `~/.codex/plugins/cache/remindb/remindb/<version>/`; the marketplace registration lives in `~/.codex/config.toml`.

Confirm the server is connected by launching Codex and running the `/mcp` slash command in the TUI:

```
/mcp
```

You should see `remindb` listed with the full `Memory*` tool suite. (The `codex mcp` CLI subcommand only manages *external* MCP servers added via `codex mcp add`; plugin-bundled MCP servers surface only inside the TUI.)

#### Seed remaining context

Step 2 compiled `~/.codex/memories/` — Codex's cross-session notes. The current project's `AGENTS.md` and in-repo docs (`README.md`, design notes, roadmaps) live in the repo, not under that path. Ask Codex in your first session to fold them in. Use absolute paths — `MemoryCompile` doesn't expand `~`:

```
remindb__MemoryCompile(path="/home/you/code/my-project/AGENTS.md", message="seed: project rules")
remindb__MemoryCompile(path="/home/you/code/my-project/README.md", message="seed: project overview")
```

Re-run whenever a file changes.

## Skills

The agent-side skills (`remind` for reads, `memoize` for writes) teach Codex how to call the `Memory*` tools effectively. Install them through [`vercel-labs/skills`](https://github.com/vercel-labs/skills):

```bash
npx skills@latest add radimsem/remindb/skills -a codex
```

Refresh them later — independent of `remindb update` — with:

```bash
npx skills@latest update
```

## Tools exposed

The plugin surfaces the full `remindb` `Memory*` tool suite under the `remindb__` namespace. See the [main README](https://github.com/radimsem/remindb#mcp-tools) for the canonical tool list and per-tool token-savings benchmarks.

## License

MIT — same as remindb.
