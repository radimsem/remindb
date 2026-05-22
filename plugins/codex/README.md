# remindb for Codex

Drops [remindb](https://github.com/radimsem/remindb) into OpenAI Codex as an MCP server. The agent picks up the full `remindb__Memory*` tool suite, backed by a compiled SQLite view of whatever workspace you point it at.

## How it works

Codex treats the repository as a marketplace catalog (`.agents/plugins/marketplace.json` at the repo root) that lists one plugin, `plugins/codex/remindb/`. The plugin's `.codex-plugin/plugin.json` points at sibling `.mcp.json`, which Codex uses to spawn `remindb serve` over stdio.

All tool logic lives in the Go binary; the plugin is a thin wrapper.

## Installation

Setup is **config-first and wizard-driven**. One Codex-specific fact shapes it: plugin-bundled MCP entries do no env-var expansion, so the durable home for the workspace paths is a **`[mcp_servers.remindb]` block in `~/.codex/config.toml`** — which both registers the server and carries its env. The wizard writes that block, so on Codex the wizard step *is* the server registration; the marketplace plugin is an alternative, not an extra requirement.

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

Pulls the `remindb-setup` wizard plus the `remind` (read) and `memoize` (write) skills:

```bash
npx skills@latest add radimsem/remindb/skills -a codex
```

Refresh later — independent of `remindb update` — with `npx skills@latest update`.

### 3. Run the setup wizard

Codex surfaces skills through the **`/skills` picker or a `$remindb-setup` mention** — not as a `/remindb-setup` slash command. Pick `remindb-setup` from `/skills`, or type `$remindb-setup`. (Manual fallback: open the installed `remindb-setup` `SKILL.md` and follow it by hand.)

The wizard detects the host, authors `.remindb/` **before** compiling, runs `remindb compile`, offers to seed adjacent context (this project's `AGENTS.md` and in-repo `README.md`), and writes the `~/.codex/config.toml` server block with your paths (see [Durable env](#durable-env)). A natural source for Codex is its own persistent context at `~/.codex/memories/` — the wizard will suggest it.

### 4. Restart and verify

The `config.toml` block the wizard wrote registers `remindb` as a user-defined MCP server, so just restart Codex. Confirm in the TUI:

```
/mcp
```

You should see `remindb` with the full `Memory*` tool suite. (The `codex mcp` CLI manages only *external* servers added via `codex mcp add`; plugin-bundled and config-defined servers surface inside the TUI.) Re-run the wizard via `/skills` — with the server attached it runs its verify pass (`MemoryStats` + `remindb://doctor`).

**Prefer the bundled plugin instead?** `codex plugin marketplace add radimsem/remindb` installs it (the marketplace's `INSTALLED_BY_DEFAULT` policy installs in the same step). But the plugin entry can't take env, so you must export `REMINDB_DB`/`REMINDB_SOURCE` in the launching shell — the `config.toml` path above avoids that.

## Durable env

`remindb serve` reads `REMINDB_DB` and `REMINDB_SOURCE` as fallbacks for its `--db`/`--source` flags. The wizard writes them into a top-level `[mcp_servers.remindb]` block in `~/.codex/config.toml`:

```toml
[mcp_servers.remindb]
command = "remindb"
args = ["serve"]
env = { REMINDB_DB = "/home/you/.cache/remindb/codex.db", REMINDB_SOURCE = "/home/you/.codex/memories" }
```

Use absolute paths — `config.toml` does not expand `~` or `$VAR`. Codex's `[plugins.<name>]` table accepts only `enabled` and does no `${VAR}` expansion in `config.toml` or the bundled `.mcp.json`, which is why the user-server block is the durable choice.

## Tools exposed

The plugin surfaces the full `remindb` `Memory*` tool suite under the `remindb__` namespace. See the [main README](https://github.com/radimsem/remindb#mcp-tools) for the canonical tool list and per-tool token-savings benchmarks.

## License

MIT — same as remindb.
