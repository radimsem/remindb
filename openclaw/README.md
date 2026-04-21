# remindb Plugin for OpenClaw

Mounts the [remindb](https://github.com/radimsem/remindb) MCP server as a workspace memory backend for OpenClaw agents.

Agents get eight `remindb__*` tools — `MemoryFetch`, `MemorySearch`, `MemoryWrite`, `MemoryCompile`, `MemoryDelta`, `MemorySummarize`, `MemoryHistory`, `MemoryTree` — backed by a compiled SQLite view of the workspace.

## How it works

The plugin ships a bundle MCP config (`.mcp.json`) that OpenClaw merges into its effective `mcpServers`. When the gateway starts, OpenClaw spawns `remindb serve` over stdio. All tool logic lives in the Go binary; the plugin is a thin wrapper.

Tools are namespaced by OpenClaw on load, so `MemoryFetch` becomes `remindb__MemoryFetch` in the agent's tool list.

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

### 2. Compile your workspace

remindb needs a SQLite file populated from your workspace before the agent can read from it:

```bash
remindb compile /path/to/workspace --db /path/to/workspace.db
```

Re-run whenever you want a fresh baseline; `serve` keeps the DB current after that.

### 3. Install the plugin

Via OpenClaw CLI:

```bash
openclaw plugins install ./openclaw
```

Or manually:

```bash
mkdir -p ~/.openclaw/extensions/remindb
cp openclaw/index.ts openclaw/openclaw.plugin.json openclaw/.mcp.json ~/.openclaw/extensions/remindb/
```

### 4. Export the workspace env vars

`remindb serve` reads two env vars as fallbacks for its `--db` and `--source` flags. Export them in the shell that launches OpenClaw so the spawned subprocess inherits them:

```bash
export REMINDB_DB=/absolute/path/to/workspace.db
export REMINDB_SOURCE=/absolute/path/to/workspace
```

Put them in `~/.bashrc` / `~/.zshrc` / fish equivalent to make the mapping permanent, or scope them to a single session to switch workspaces between runs. Re-export and restart the gateway whenever the agent should target a different workspace.

### 5. Restart the gateway

```bash
openclaw gateway restart
```

## Configuration

Alternatively enable the plugin and pin its config in `openclaw.json`:

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
