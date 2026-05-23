# remindb for Hermes Agent

Makes [remindb](https://github.com/radimsem/remindb) the memory backend for [Hermes Agent](https://hermes-agent.nousresearch.com). Once installed, the agent gets the full `Memory*` tool suite (17 tools) over a compiled SQLite view of whatever workspace you point it at.

## How it works

Hermes memory plugins are Python classes that implement the [`MemoryProvider` ABC](https://hermes-agent.nousresearch.com/docs/developer-guide/memory-provider-plugin). This one spawns `remindb serve` as a subprocess and forwards Hermes' tool calls to the MCP server as `tools/list` and `tools/call` requests over stdio. The Python layer is a thin bridge; the actual tool logic lives in the `remindb` binary.

> **Hermes runs one external memory provider at a time** (set by `memory.provider` in `~/.hermes/config.yaml`). Installing this plugin makes remindb that provider for the current profile.

> [!IMPORTANT]
> **Hermes is outside the unified MCP-host setup flow.** The other five plugins ([Claude Code](../../../claude-code/), [Gemini CLI](../../../gemini-cli/), [Codex](../../../codex/), [OpenCode](../../../opencode/), [OpenClaw](../../../openclaw/)) expose remindb as an MCP server wired through an `env` block, and are configured by the `/remindb-setup` skill wizard. Hermes is a **memory-provider bridge**, not an MCP-config host: it has no `mcpServers` `env` block and is set up with its own **`hermes memory setup`** wizard writing **`remindb.json`**. Don't run `/remindb-setup` here, and don't look for an `env` block to edit — follow the steps below instead. (You can still install the `remind`/`memoize` companion skills; see [Skills](#skills).)

## Installation

### 1. Install the remindb binary

It needs to be on your `$PATH`:

```bash
curl -fsSL https://raw.githubusercontent.com/radimsem/remindb/main/install.sh | bash
```

On Windows:

```powershell
iwr -useb https://raw.githubusercontent.com/radimsem/remindb/main/install.ps1 | iex
```

Check it:

```bash
remindb --version
```

The binary is a hard prerequisite — `hermes memory setup` configures paths but does not install it. If it's missing, the plugin raises a clear error pointing back at the URL above the first time Hermes touches memory.

### 2. Compile a source directory

remindb reads from a SQLite file built out of a source tree, so compile one first.

A natural choice for Hermes is its own profile data under `~/.hermes/`: the markdown notes, profile config, and session logs it accumulates.

```bash
mkdir -p ~/.cache/remindb
remindb compile ~/.hermes --db ~/.cache/remindb/hermes.db
```

Any workspace works, though. Point it at a docs tree, a notes vault, or a project you want Hermes to remember.

### 3. Install the plugin

`hermes plugins install <repo>` clones the whole repo and expects `plugin.yaml` at its root — it has no `--path` flag and can't scope to a subdirectory, so the plugin can't be installed that way from this monorepo today. Clone the repo and copy the plugin directory into `$HERMES_HOME/plugins/` instead (`$HERMES_HOME` defaults to `~/.hermes`):

```bash
git clone https://github.com/radimsem/remindb.git
mkdir -p ~/.hermes/plugins
rm -rf ~/.hermes/plugins/remindb
cp -r remindb/plugins/hermes-agent/memory/remindb ~/.hermes/plugins/
```

Memory providers don't need `hermes plugins enable` — Hermes discovers them by scanning `~/.hermes/plugins/` and activates them via `memory.provider` in `~/.hermes/config.yaml` (the next step). Update later by re-running the last two commands (the `rm -rf` makes it deletion-safe — a removed plugin file won't linger).

### 4. Configure remindb as the memory provider

```bash
hermes memory setup
```

The wizard asks for two paths:
- `REMINDB_DB` — the file you compiled in step 2.
- `REMINDB_SOURCE` — the directory you compiled it from.

It writes both to `$HERMES_HOME/remindb.json` and sets `memory.provider: remindb` in `~/.hermes/config.yaml`.

Prefer environment variables? Set them instead. They take precedence over `remindb.json`:

```bash
export REMINDB_DB=$HOME/.cache/remindb/hermes.db
export REMINDB_SOURCE=$HOME/.hermes
```

### 5. Verify

Start Hermes. The next time it touches memory, it spawns `remindb serve` and loads all 17 `Memory*` tools. They show up in any session that uses the memory layer. Ask the agent to list its memory tools, or have it call `MemoryStats` once to confirm.

If something looks off, run `remindb serve` yourself in a terminal with the same `REMINDB_DB` and `REMINDB_SOURCE` set. You'll see its logs directly.

### Troubleshooting

- `hermes memory status` shows `Status: not available ✗` after setup — check `remindb --version`; if it's not on `$PATH`, re-run step 1. The first time Hermes touches memory it raises `remindb binary not found on PATH. Install with: …` pointing at the installer.
- Wizard saved with empty paths — re-run `hermes memory setup`, or set `REMINDB_DB` / `REMINDB_SOURCE` as env vars (they take precedence over `remindb.json`).
- Smoke test the plugin locally: `python3 plugins/hermes-agent/memory/remindb/test-smoke.py` (uses a throwaway `HERMES_HOME`, leaves your real `~/.hermes/` untouched).

## Skills

Two companion skills teach Hermes how to drive the tools well: `remind` for reads, `memoize` for writes. Install them with [`vercel-labs/skills`](https://github.com/vercel-labs/skills):

```bash
npx skills@latest add radimsem/remindb/skills -a hermes-agent
```

Update them later with:

```bash
npx skills@latest update
```

## Tools

The plugin exposes the full `Memory*` suite. The [remindb project page](https://github.com/radimsem/remindb#mcp-tools) lists every tool and its measured token savings.

## Behavior worth knowing

- The `remindb serve` subprocess starts when Hermes initializes memory and shuts down when the session ends. It shares Hermes' process group, so it dies with Hermes even on a crash.
- Tool schemas are pulled live at startup rather than hard-coded, so new `Memory*` tools in a future remindb release appear automatically — no plugin update needed.

## License

MIT.
