# Host wiring — install command, durable env, and invocation per host

Reference for `remindb-setup`. Load in **Pass 1 step 2** (after detecting the host) and step 7 (wiring the env). One row per supported MCP host: how the agent-side skill is installed, how `/remindb-setup` is actually invoked there, the **durable** mechanism for the `REMINDB_DB` / `REMINDB_SOURCE` env, and how the MCP plugin itself is installed.

The wizard's job in step 7 is to **apply the durable env mechanism itself** wherever it can write a host-owned config file; it falls back to emitting a copy-paste snippet only in the two cases flagged below (overwrite-on-update marketplace cache, unknown host).

Hermes Agent is **not** in this table — it's a memory-provider bridge with its own `hermes memory setup` wizard and a `remindb.json` config, not an MCP `env` block. See its plugin README; don't wire it here.

## Matrix

| Host | Skill install | Invoke the wizard | Durable env mechanism (what the wizard writes) | Plugin install |
|---|---|---|---|---|
| **Claude Code** | `npx skills@latest add radimsem/remindb/skills -a claude-code` | `/remindb-setup` (slash command) | **Shell export** — the marketplace `.mcp.json` is overwritten on every plugin update, so it can't durably hold paths. The wizard **emits a snippet** (export `REMINDB_DB`/`REMINDB_SOURCE` in the shell rc); the `.mcp.json` `${VAR}` passthroughs pick them up. Local-checkout installs (`--plugin-dir`) own the file → the wizard can edit its `env` block directly. | `/plugin marketplace add radimsem/remindb` → `/plugin install remindb@remindb` (or `--plugin-dir ./plugins/claude-code`) |
| **Codex** | `npx skills@latest add radimsem/remindb/skills -a codex` | `/skills` picker or `$remindb-setup` mention — **not** `/remindb-setup` (Codex skills aren't slash commands) | **`~/.codex/config.toml`** — the wizard writes a top-level `[mcp_servers.remindb]` block with the paths in its `env` table (registers remindb as a user MCP server). Plugin `.mcp.json` does no `${VAR}` expansion, so this is the only durable place. | `codex plugin marketplace add radimsem/remindb` (installs by default) |
| **OpenCode** | `npx skills@latest add radimsem/remindb/skills -a opencode` | Ask by name in plain language ("use the remindb-setup skill") — no slash command for skills | **`opencode.json`** — the wizard merges/writes the `mcp.remindb.environment` block, using `{env:HOME}` expansion (OpenCode expands only `{env:VAR}`, never `$HOME`/`${HOME}`). Project-level preferred; global at `~/.config/opencode/opencode.json`. | merge the `mcp.remindb` entry into `opencode.json` (no separate install step) |
| **OpenClaw** | `npx skills@latest add radimsem/remindb/skills -a openclaw` | `/remindb-setup` (slash command; `user-invocable` defaults true) | **`openclaw mcp set`** — the wizard runs `openclaw mcp set remindb '{…,"env":{…}}'` (env passed straight to the spawned subprocess), then advises `openclaw gateway restart`. Also add the bare id `"remindb"` to each agent's `tools.allow` in `~/.openclaw/openclaw.json`. | `openclaw plugins install ./plugins/openclaw` (from a clone) |
| **Gemini CLI** | `npx skills@latest add radimsem/remindb/skills -a gemini-cli` | Plain language ("run the remindb setup wizard") → `activate_skill` — no `/remindb-setup` command (skills are model-activated; slash commands are a separate TOML subsystem) | **`gemini-extension.json`** — you install from a local clone, so the wizard writes the two paths into that clone's manifest `env` block; re-install (`uninstall` + `install`) to apply. | clone, then `gemini extensions install <clone>/plugins/gemini-cli` |

## Invocability — the practical consequence

Only **Claude Code** and **OpenClaw** expose `/remindb-setup` as a real slash command. On **Codex**, **OpenCode**, and **Gemini CLI** the skill is loaded but reached by the skill picker / a `$mention` / plain-language activation, not a `/` command. This does not change the wizard's behavior — once invoked, the SKILL.md body runs identically. It only changes the *invocation line* a host README should print, and means a README for a non-slash-command host must keep a one-line manual fallback: "or open the installed `remindb-setup` SKILL.md and follow it by hand."

The `user-invocable: true` in the wizard frontmatter guarantees the Claude Code / OpenClaw exposure rather than relying on each host's implicit default.

## Snippet-fallback cases (the only two)

The wizard prefers to write host config itself. It drops to a copy-paste snippet only when it **can't durably write**:

1. **Claude Code marketplace install** — `~/.claude/plugins/.../.mcp.json` is regenerated on every plugin update, so any edit is transient. Emit the shell-export snippet instead (and note the local-checkout path can be edited durably).
2. **Unknown / undetected host** — no known durable mechanism. Emit a generic `mcpServers` `env` snippet (the shape in the top-level README) and tell the user where their host reads MCP config.
