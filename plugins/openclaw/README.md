# remindb Plugin for OpenClaw

Mounts the [remindb](https://github.com/radimsem/remindb) MCP server as a workspace memory backend for OpenClaw agents.

Agents get the full `remindb__Memory*` tool suite — backed by a compiled SQLite view of the workspace.

## How it works

The plugin ships a bundle MCP config (`.mcp.json`) that OpenClaw merges into its effective `mcpServers`. When the gateway starts, OpenClaw spawns `remindb serve` over stdio. All tool logic lives in the Go binary; the plugin is a thin wrapper.

Tools are namespaced by OpenClaw on load, so `MemoryFetch` becomes `remindb__MemoryFetch` in the agent's tool list.

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

### 2. Compile your workspace

remindb needs a SQLite file populated from your workspace before the agent can read from it:

```bash
remindb compile /path/to/workspace --db /path/to/workspace.db
```

Re-run whenever you want a fresh baseline; `serve` keeps the DB current after that.

### 3. Install the plugin

Via OpenClaw CLI:

```bash
openclaw plugins install ./plugins/openclaw
```

Or manually:

```bash
mkdir -p ~/.openclaw/extensions/remindb
cp plugins/openclaw/index.ts plugins/openclaw/openclaw.plugin.json plugins/openclaw/.mcp.json ~/.openclaw/extensions/remindb/
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

The plugin surfaces the full `remindb` `Memory*` tool suite under the `remindb__` namespace. See the [main README](https://github.com/radimsem/remindb#mcp-tools) for the canonical tool list and token-savings benchmarks per tool.

## License

MIT — same as remindb.
