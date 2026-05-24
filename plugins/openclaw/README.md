# remindb for OpenClaw

Drops [remindb](https://github.com/radimsem/remindb) into OpenClaw as an MCP server. The agent picks up the full `remindb__Memory*` tool suite, backed by a compiled SQLite view of whatever workspace you point it at.

## How it works

OpenClaw splits plugin registration from MCP server wiring. The plugin (`index.ts` + `openclaw.plugin.json`) declares itself as an extension; the MCP server is registered separately at the gateway level via `openclaw mcp set`. When the gateway starts, OpenClaw spawns `remindb serve` over stdio. All tool logic lives in the Go binary; the plugin is a thin wrapper.

Tools are namespaced, so `MemoryFetch` becomes `remindb__MemoryFetch`. Each agent's `tools.allow` array in `~/.openclaw/openclaw.json` must include the bare plugin id `"remindb"` — OpenClaw expands it to all `remindb__*` tools at policy time.

## Installation

Setup is **config-first and wizard-driven**: the `remindb-setup` skill authors `.remindb/`, compiles, and wires the server env (via `openclaw mcp set`) for you. The plugin extension and the `tools.allow` grant are separate gateway-level steps that follow.

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

Pulls the `remindb-setup` wizard plus the `remind` (read) and `memorize` (write) skills:

```bash
npx skills@latest add radimsem/remindb/skills -a openclaw
```

Refresh later — independent of `remindb update` — with `npx skills@latest update`.

### 3. Run the setup wizard

OpenClaw exposes the skill as a slash command:

```
/remindb-setup            # interactive
/remindb-setup automode   # hands-off
```

The wizard detects the host, authors `.remindb/` **before** compiling, runs `remindb compile`, offers to seed adjacent context (this project's `AGENTS.md`/`SOUL.md`/`README.md`), and registers the server with your paths via `openclaw mcp set` (see [Durable env](#durable-env)). A natural source for OpenClaw is its own state folder at `~/.openclaw/` — the wizard will suggest it and exclude the session transcripts, secrets, and runtime state under it.

### 4. Install the plugin and grant the tools

The extension installs from a clone:

```bash
git clone https://github.com/radimsem/remindb.git ~/code/remindb   # or: git -C ~/code/remindb checkout v0.1.0
openclaw plugins install ~/code/remindb/plugins/openclaw
```

Then add the bare id `"remindb"` to each agent's `tools.allow` array in `~/.openclaw/openclaw.json` (a manual JSON edit per agent — without it the plugin loads but no `remindb__*` tools appear).

### 5. Restart the gateway and verify

```bash
openclaw gateway restart
openclaw mcp list
openclaw plugins inspect remindb
openclaw config validate
```

You should see `remindb` with the full `Memory*` tool suite. Re-run `/remindb-setup` — with the server attached it runs its verify pass (`MemoryStats` + `remindb://doctor`).

## Durable env

`remindb serve` reads `REMINDB_DB` and `REMINDB_SOURCE` as fallbacks for its `--db`/`--source` flags. The wizard registers the server with the paths in the `env` object — passed straight to the spawned subprocess:

```bash
openclaw mcp set remindb '{"command":"remindb","args":["serve"],"env":{"REMINDB_DB":"/abs/path/workspace.db","REMINDB_SOURCE":"/abs/path/workspace"}}'
```

`openclaw mcp set` **writes the gateway's MCP config to disk** — it doesn't require a running gateway, which is why the wizard can run it in step 3 before the plugin and gateway are up. The registration takes effect on the next gateway start, so the step-5 `openclaw gateway restart` is what applies it (along with the plugin and `tools.allow` grant from step 4). Swap the paths and re-run `openclaw mcp set` (then restart the gateway) to target a different workspace. Equivalently, pass `--db`/`--source` as `args` for per-server pinning:

```bash
openclaw mcp set remindb '{"command":"remindb","args":["serve","--db","/abs/path/workspace.db","--source","/abs/path/workspace"]}'
```

## Tools exposed

The plugin surfaces the full `remindb` `Memory*` tool suite under the `remindb__` namespace. See the [main README](https://github.com/radimsem/remindb#mcp-tools) for the canonical tool list and per-tool token-savings benchmarks.

## License

MIT — same as remindb.
