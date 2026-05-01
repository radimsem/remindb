# remindb for OpenClaw

Drops [remindb](https://github.com/radimsem/remindb) into OpenClaw as an MCP server. The agent picks up the full `remindb__Memory*` tool suite, backed by a compiled SQLite view of whatever workspace you point it at.

## How it works

The plugin ships a bundle MCP config (`.mcp.json`) that OpenClaw merges into its effective `mcpServers`. When the gateway starts, OpenClaw spawns `remindb serve` over stdio. All tool logic lives in the Go binary; the plugin is a thin wrapper.

Tools are namespaced by OpenClaw on load, so `MemoryFetch` becomes `remindb__MemoryFetch` in the agent's tool list.

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

### 2. Compile your workspace or agent state folder

remindb needs a SQLite file built from a source tree before the agent can read from it.

A natural source for OpenClaw is its own state folder at `~/.openclaw/` — `openclaw.json`, hook scripts under `hooks/<id>/`, agent workspaces under `workspace/` (and `workspace-*`), per-agent state under `agents/<id>/`, installed plugins under `extensions/`, and shared skill definitions under `skills/`. Indexing it lets OpenClaw query its own persistent context through remindb instead of grepping the dot folder.

`~/.openclaw/` also accumulates session transcripts at `agents/<id>/sessions/*.jsonl`, OAuth + API-key stores at `agents/<id>/agent/auth-profiles.json` (with provider apiKey residues sometimes spilling into adjacent `models.json`), the `extensions/` plugin install dir (you don't want remindb indexing its own bundled `index.ts` / `.mcp.json`), and `sandboxes/` / `sandbox/` runtime state. Drop a `.remindb.ignore` at `~/.openclaw/` to filter them out (gitignore-style minimal subset: `*`, `**`, trailing `/`, `#` comments; no `!` negation, no `[abc]` ranges):

```bash
mkdir -p ~/.cache/remindb
printf '%s\n' \
    '# Compile only curated context; skip session transcripts, secrets, and runtime state.' \
    '' \
    '# Session transcripts.' \
    '*.jsonl' \
    '# Per-agent session subtrees (agents/<id>/sessions/).' \
    '**/sessions/' \
    '# OAuth and API-key store (agents/<id>/agent/auth-profiles.json).' \
    '**/auth-profiles.json' \
    '# Provider apiKey residues sometimes leak into agents/<id>/agent/models.json.' \
    '**/models.json' \
    '# Installed plugins — avoid indexing remindb plugin source.' \
    'extensions/' \
    '# Sandbox runtime state.' \
    'sandboxes/' \
    '# Sandbox config (containers.json).' \
    'sandbox/' \
    > ~/.openclaw/.remindb.ignore
remindb compile ~/.openclaw --db ~/.cache/remindb/openclaw.db
```

The same `.remindb.ignore` is honored by `serve`'s background rescan and the `MemoryCompile` tool — set it once, all paths agree. Or point at any other workspace you want the agent to see:

```bash
remindb compile /path/to/workspace --db /path/to/workspace.db
```

Re-run `compile` whenever you want a fresh baseline; `serve` keeps the DB current after that.

### 3. Point remindb at your workspace

`remindb serve` reads two env vars as fallbacks for its `--db` and `--source` flags. Export them in the shell **before launching OpenClaw with the plugin enabled** — otherwise the first activation falls back to a stray `memory.db` in cwd:

```bash
export REMINDB_DB=/absolute/path/to/workspace.db
export REMINDB_SOURCE=/absolute/path/to/workspace
```

Stick them in `~/.bashrc` / `~/.zshrc` / your fish equivalent to make it permanent, or scope to a single session if you want to switch workspaces between runs. Re-export and restart the gateway whenever the agent should target a different workspace.

### 4. Install the plugin

Both install paths point at a local checkout, so clone the repo first:

```bash
git clone https://github.com/radimsem/remindb.git ~/code/remindb
cd ~/code/remindb
```

Pin to a release tag if you want a stable version: `git -C ~/code/remindb checkout v0.1.0`.

Via OpenClaw CLI:

```bash
openclaw plugins install ./plugins/openclaw
```

Or by hand:

```bash
mkdir -p ~/.openclaw/extensions/remindb
cp plugins/openclaw/index.ts plugins/openclaw/openclaw.plugin.json plugins/openclaw/.mcp.json ~/.openclaw/extensions/remindb/
```

### 5. Restart the gateway

```bash
openclaw gateway restart
```

#### Seed remaining context

Step 2 compiled `~/.openclaw/` — OpenClaw's own state folder. The current project's `AGENTS.md` (or `SOUL.md`, `USER.md`, `MEMORY.md`) and in-repo docs (`README.md`, design notes, roadmaps) live in the repo, not under that path. Ask the agent in your first session to fold them in. Use absolute paths — `MemoryCompile` doesn't expand `~`:

```
remindb__MemoryCompile(path="/home/you/code/my-project/AGENTS.md", message="seed: project rules")
remindb__MemoryCompile(path="/home/you/code/my-project/README.md", message="seed: project overview")
```

Re-run whenever a file changes.

## Configuration

You can also enable the plugin and pin its config in `openclaw.json`:

```json5
{
  plugins: {
    entries: {
      "remindb": {
        enabled: true
      }
    }
  }
}
```

The plugin itself has no runtime options. `remindb serve` resolves its DB and source paths from `REMINDB_DB` and `REMINDB_SOURCE` at launch; explicit `--db` / `--source` flags in `.mcp.json` override the env vars if you need per-bundle pinning.

## Tools exposed

The plugin surfaces the full `remindb` `Memory*` tool suite under the `remindb__` namespace. See the [main README](https://github.com/radimsem/remindb#mcp-tools) for the canonical tool list and per-tool token-savings benchmarks.

## License

MIT — same as remindb.
