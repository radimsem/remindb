# remindb for OpenCode

Drops [remindb](https://github.com/radimsem/remindb) into OpenCode as an MCP server. The agent picks up the full `remindb__Memory*` tool suite, backed by a compiled SQLite view of whatever workspace you point it at.

## How it works

OpenCode configures MCP servers in `opencode.json` under the top-level `mcp` object rather than via a plugin API. This folder ships:

- `opencode.json` — a ready-to-merge MCP entry that spawns `remindb serve` over stdio.
- `plugin.ts` — a minimal OpenCode plugin stub so the bundle can be distributed as an npm package for users who prefer that path.

## Installation

Setup is **config-first and wizard-driven**. One OpenCode-specific fact shapes it: the server lives in `opencode.json` (there's no separate plugin install), and its `environment` block is the **durable home** for the workspace paths. The wizard writes that entry, so on OpenCode the wizard step *is* the server registration.

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
npx skills@latest add radimsem/remindb/skills -a opencode
```

Refresh later — independent of `remindb update` — with `npx skills@latest update`.

### 3. Run the setup wizard

OpenCode loads skills **by name in conversation, not as a slash command** — ask for it:

```
use the remindb-setup skill
```

(Manual fallback: open the installed `remindb-setup` `SKILL.md` and follow it by hand.) The wizard detects the host, authors `.remindb/` **before** compiling, runs `remindb compile`, offers to seed adjacent context (the global `~/.config/opencode/AGENTS.md`, ancestor `AGENTS.md` files, and the `~/.claude/CLAUDE.md` fallback), and merges the `mcp.remindb` entry into `opencode.json` with your paths (see [Durable env](#durable-env)). It prefers a project-level `opencode.json` so each workspace carries its own paths.

### 4. Restart and verify

OpenCode reads `opencode.json` on session start, so launch a fresh session, then:

```bash
opencode mcp list
```

You should see `remindb` with the full `Memory*` tool suite. Ask for the `remindb-setup` skill again — with the server attached it runs its verify pass (`MemoryStats` + `remindb://doctor`).

## Durable env

`remindb serve` reads `REMINDB_DB` and `REMINDB_SOURCE` as fallbacks for its `--db`/`--source` flags. The wizard writes them into the `environment` block of the `mcp.remindb` entry — OpenCode passes it straight to the spawned subprocess:

```json
{
    "$schema": "https://opencode.ai/config.json",
    "mcp": {
        "remindb": {
            "type": "local",
            "command": ["remindb", "serve"],
            "environment": {
                "REMINDB_DB": "{env:HOME}/.cache/remindb/my-project.db",
                "REMINDB_SOURCE": "{env:HOME}/code/my-project"
            },
            "enabled": true
        }
    }
}
```

OpenCode expands **only** `{env:VAR}` in config values — literal `$HOME`/`${HOME}` won't work. Project-level `opencode.json` lives at the repo root; global at `~/.config/opencode/opencode.json`. To list the bundle in OpenCode's plugin UI, also reference the npm stub: `"plugin": ["@radimsem/remindb-opencode"]` (OpenCode runs `bun install` at startup to resolve it).

## Tools exposed

The plugin surfaces the full `remindb` `Memory*` tool suite under the `remindb__` namespace. See the [main README](https://github.com/radimsem/remindb#mcp-tools) for the canonical tool list and per-tool token-savings benchmarks.

## License

MIT — same as remindb.
