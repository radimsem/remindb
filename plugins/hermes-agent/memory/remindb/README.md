# remindb for Hermes Agent

Makes [remindb](https://github.com/radimsem/remindb) the memory backend for [Hermes Agent](https://hermes-agent.nousresearch.com). Once installed, the agent gets the full `Memory*` tool suite (17 tools) over a compiled SQLite view of whatever workspace you point it at.

## How it works

Hermes memory plugins are Python classes that implement the [`MemoryProvider` ABC](https://hermes-agent.nousresearch.com/docs/developer-guide/memory-provider-plugin). This one spawns `remindb serve` as a subprocess and forwards Hermes' tool calls to the MCP server as `tools/list` and `tools/call` requests over stdio. The Python layer is a thin bridge; the actual tool logic lives in the `remindb` binary.

> **Hermes runs one external memory provider at a time** (set by `memory.provider` in `~/.hermes/config.yaml`). Installing this plugin makes remindb that provider for the current profile.

> [!IMPORTANT]
> **Hermes is a bridge host, not a unified-flow MCP host.** The other five plugins ([Claude Code](../../../claude-code/), [Gemini CLI](../../../gemini-cli/), [Codex](../../../codex/), [OpenCode](../../../opencode/), [OpenClaw](../../../openclaw/)) wire remindb through an MCP `env` block, and `/remindb-setup` runs the full pipeline (`.remindb/` → compile → plugin install → env wire). Here, **`hermes memory setup`** owns env wiring via **`remindb.json`** — so you run `/remindb-setup` in **`only-config`** mode (step 3 below) for `.remindb/` authoring only, and Hermes' own wizard does the rest.

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

### 2. Install the companion skills

This pulls `remindb-setup` (used scoped in step 3), `remind` (read tools), and `memoize` (write tools) via [`vercel-labs/skills`](https://github.com/vercel-labs/skills):

```bash
npx skills@latest add radimsem/remindb/skills -a hermes-agent
```

Refresh later — independent of `remindb update` — with `npx skills@latest update`.

### 3. Author `.remindb/` via `/remindb-setup only-config`

In a Hermes Agent session, paste:

```
/remindb-setup only-config

Source root: ~/.hermes/memories
Also adjacent-seed: ~/.hermes/SOUL.md
Stop after authoring `.remindb/` — Hermes Agent owns env wiring via `hermes memory setup`.
```

The wizard runs **steps 1–4 of its Pass 1 only**: it confirms `remindb` is on `$PATH`, identifies Hermes, settles `~/.hermes/memories/` as the source, then writes the four `.remindb/` files at `~/.hermes/memories/.remindb/` (`ignore`, `pinned`, `temperatures.json`, `config.json`) using the "Memory-provider bridge — Hermes Agent" template. It closes with a one-line summary: `SOUL.md` is queued for the next step's adjacent-seed; compile, plugin install, and env wiring are deferred to Hermes' own pipeline (steps 4 and 6 below).

### 4. Compile

Build the SQLite the running server will read. Source is `~/.hermes/memories/` (Hermes' own curated memory directory); then seed `SOUL.md` adjacently — it lives at HERMES_HOME root, outside the source.

```bash
mkdir -p ~/.cache/remindb
remindb compile ~/.hermes/memories --db ~/.cache/remindb/hermes.db
remindb compile ~/.hermes/SOUL.md --db ~/.cache/remindb/hermes.db
```

> ℹ️ **`SOUL.md` is a one-shot adjacent seed.** The rescan loop (started by `remindb serve`) only watches the source root — here, `~/.hermes/memories/`. Files compiled from outside that source, like `SOUL.md`, are ingested but not auto-refreshed. If Hermes updates `SOUL.md` later (e.g. via its `soul_edit` tool), re-run the second `remindb compile ~/.hermes/SOUL.md ...` command to refresh the stored copy.

> ⚠️ **Source is `~/.hermes/memories/`, not `~/.hermes/` wholesale.** HERMES_HOME also holds installed plugins (`plugins/`), agent-created skills (`skills/`), gateway sessions (`sessions/`), logs (`logs/`), caches (`cache/`), sandboxes (`sandboxes/`), browser recordings (`browser_recordings/`), and credential files (`.env`, `auth.json`) — none of which are recall-shaped. The narrow source root plus the `.remindb/` template authored in step 3 keep the index to genuine memory only. (The exclusion list reflects the Hermes HERMES_HOME layout at time of writing; if a newer Hermes release adds a memory-adjacent subtree, verify against current Hermes docs.)

Any other source root works if you want to point Hermes at a different workspace (a docs vault, a project tree) — adjust step 3's `Source root:` accordingly. For the canonical "remember your own state" case, the narrow `memories/` source above is the right shape.

### 5. Install the plugin

`hermes plugins install <repo>` clones the whole repo and expects `plugin.yaml` at its root — it has no `--path` flag and can't scope to a subdirectory, so the plugin can't be installed that way from this monorepo today. Clone the repo into the same `~/.cache/remindb/` directory that holds the compiled DB, then copy the plugin subdir into `$HERMES_HOME/plugins/` (`$HERMES_HOME` defaults to `~/.hermes`):

```bash
mkdir -p ~/.cache/remindb
git clone https://github.com/radimsem/remindb.git ~/.cache/remindb/src
mkdir -p ~/.hermes/plugins
rm -rf ~/.hermes/plugins/remindb
cp -r ~/.cache/remindb/src/plugins/hermes-agent/memory/remindb ~/.hermes/plugins/
```

The clone lives at `~/.cache/remindb/src` — alongside the DB, out of your working directories. Update later by pulling in place and re-copying:

```bash
git -C ~/.cache/remindb/src pull
rm -rf ~/.hermes/plugins/remindb
cp -r ~/.cache/remindb/src/plugins/hermes-agent/memory/remindb ~/.hermes/plugins/
```

Memory providers don't need `hermes plugins enable` — Hermes discovers them by scanning `~/.hermes/plugins/` and activates them via `memory.provider` in `~/.hermes/config.yaml` (the next step). The `rm -rf` before each copy makes the install deletion-safe — a file removed upstream won't linger in your installed copy.

### 6. Configure remindb as the memory provider

```bash
hermes memory setup
```

The wizard asks for two paths:
- `REMINDB_DB` — the file you compiled in step 4.
- `REMINDB_SOURCE` — the directory you compiled it from (`~/.hermes/memories`).

It writes both to `$HERMES_HOME/remindb.json` and sets `memory.provider: remindb` in `~/.hermes/config.yaml`. This is the env-wiring step bridge-host mode deferred to: Hermes owns it, not `/remindb-setup`.

Prefer environment variables? Set them instead. They take precedence over `remindb.json`:

```bash
export REMINDB_DB=$HOME/.cache/remindb/hermes.db
export REMINDB_SOURCE=$HOME/.hermes/memories
```

### 7. Verify

Start Hermes. The next time it touches memory, it spawns `remindb serve` and loads all 17 `Memory*` tools. They show up in any session that uses the memory layer. Ask the agent to list its memory tools, or have it call `MemoryStats` once to confirm `db_path` matches `~/.cache/remindb/hermes.db`.

If something looks off, run `remindb serve` yourself in a terminal with the same `REMINDB_DB` and `REMINDB_SOURCE` set. You'll see its logs directly.

### Troubleshooting

- `hermes memory status` shows `Status: not available ✗` after setup — check `remindb --version`; if it's not on `$PATH`, re-run step 1. The first time Hermes touches memory it raises `remindb binary not found on PATH. Install with: …` pointing at the installer.
- Wizard saved with empty paths — re-run `hermes memory setup`, or set `REMINDB_DB` / `REMINDB_SOURCE` as env vars (they take precedence over `remindb.json`).
- Smoke test the plugin locally: `python3 plugins/hermes-agent/memory/remindb/test-smoke.py` (uses a throwaway `HERMES_HOME`, leaves your real `~/.hermes/` untouched).

## Tools

The plugin exposes the full `Memory*` suite. The [remindb project page](https://github.com/radimsem/remindb#mcp-tools) lists every tool and its measured token savings.

## Behavior worth knowing

- The `remindb serve` subprocess starts when Hermes initializes memory and shuts down when the session ends. It shares Hermes' process group, so it dies with Hermes even on a crash.
- Tool schemas are pulled live at startup rather than hard-coded, so new `Memory*` tools in a future remindb release appear automatically — no plugin update needed.

## License

MIT.
