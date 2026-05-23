# Host wiring — install command, scoped env, and invocation per host

Reference for `remindb-setup`. Load in **Pass 1 step 2** (after detecting the host), step 7 (installing the MCP plugin), and step 8 (wiring the env). One row per supported MCP host: how the agent-side skill is installed, how `/remindb-setup` is actually invoked, **which file the running server actually loads** for `REMINDB_DB` / `REMINDB_SOURCE`, the scoped edit the wizard applies to it, and how the MCP plugin itself is installed.

The wizard's job in step 8 is to **write the scoped edit itself** (the loaded config the running server reads) — multi-brain safe because the env scopes to the remindb subprocess only. It falls back to a shell-export snippet only when scoped isn't viable: an unknown host, or — for Claude Code marketplace installs — when the user explicitly prefers the env to survive plugin updates and accepts the multi-session clobber. See **§Env-wiring principle** below.

Hermes Agent is **not** in this table — it's a memory-provider bridge with its own `hermes memory setup` wizard and a `remindb.json` config, not an MCP `env` block. See its plugin README; don't wire it here.

## Matrix

The fourth column names **the file the running server actually reads**, flags whether it exists pre- or post-plugin-install, and gives the scoped edit the wizard applies. Don't trust this from memory at edit time — Pass 1 step 8 verifies the file exists before writing.

| Host | Skill install | Invoke the wizard | Loaded config + env-wiring (scoped preferred) | Plugin install |
|---|---|---|---|---|
| **Claude Code** | `npx skills@latest add radimsem/remindb/skills -a claude-code` | `/remindb-setup` (slash command) | **Loaded:** versioned install cache `~/.claude/plugins/cache/<mp>/<plugin>/<version>/.mcp.json` — **post-install**, regenerated on every plugin update. The marketplace clone `~/.claude/plugins/marketplaces/<mp>/plugins/claude-code/.mcp.json` is the source — **editing the clone is a no-op for the running server**. **Scoped edit (primary):** write the cache's `env` block — multi-brain safe; lost on update (wizard re-runs to refresh). **Shell-export fallback:** `${VAR}` passthroughs in the cache pick up `REMINDB_DB`/`REMINDB_SOURCE` from the shell rc — survives updates but **clobbers every remindb session launched from that shell**. Local-checkout installs (`--plugin-dir`) own the file → scoped edit is durable, no fallback needed. | `/plugin marketplace add radimsem/remindb` → `/plugin install remindb@remindb` (or `--plugin-dir ./plugins/claude-code`) |
| **Codex** | `npx skills@latest add radimsem/remindb/skills -a codex` | `/skills` picker or `$remindb-setup` mention — **not** `/remindb-setup` (Codex skills aren't slash commands) | **Loaded:** `~/.codex/config.toml` — exists **pre-install** (user-authored). Plugin `.mcp.json` does no `${VAR}` expansion, so this is the only durable place. **Scoped edit:** write a top-level `[mcp_servers.remindb]` block with paths in its `env` table (registers remindb as a user MCP server). | `codex plugin marketplace add radimsem/remindb` (installs by default) |
| **OpenCode** | `npx skills@latest add radimsem/remindb/skills -a opencode` | Ask by name in plain language ("use the remindb-setup skill") — no slash command for skills | **Loaded:** `opencode.json` (project root preferred; else `~/.config/opencode/opencode.json`) — **pre-install** (user-authored or merge target). **Scoped edit:** merge/write the `mcp.remindb.environment` block, using `{env:HOME}` expansion (OpenCode expands only `{env:VAR}`, never `$HOME`/`${HOME}`). | merge the `mcp.remindb` entry into `opencode.json` (no separate install step) |
| **OpenClaw** | `npx skills@latest add radimsem/remindb/skills -a openclaw` | `/remindb-setup` (slash command; `user-invocable` defaults true) | **Loaded:** the OpenClaw MCP store (manipulated via `openclaw mcp set`) plus `~/.openclaw/openclaw.json` for `tools.allow` — **post-install** (the manifest registers the entry first). **Scoped edit:** `openclaw mcp set remindb '{…,"env":{…}}'` (env scoped to the spawned subprocess), then `openclaw gateway restart`; also add the bare id `"remindb"` to each agent's `tools.allow`. | `openclaw plugins install ./plugins/openclaw` (from a clone) |
| **Gemini CLI** | `npx skills@latest add radimsem/remindb/skills -a gemini-cli` | Plain language ("run the remindb setup wizard") → `activate_skill` — no `/remindb-setup` command (skills are model-activated; slash commands are a separate TOML subsystem) | **Loaded:** the clone's `gemini-extension.json` `env` block — **pre-install in the clone**, applied once installed (re-install to refresh). **Scoped edit:** write `REMINDB_DB`/`REMINDB_SOURCE` into the manifest's `env` block; `uninstall` + `install` to apply. | clone, then `gemini extensions install <clone>/plugins/gemini-cli` |

## Invocability — the practical consequence

Only **Claude Code** and **OpenClaw** expose `/remindb-setup` as a real slash command. On **Codex**, **OpenCode**, and **Gemini CLI** the skill is loaded but reached by the skill picker / a `$mention` / plain-language activation, not a `/` command. This does not change the wizard's behavior — once invoked, the SKILL.md body runs identically. It only changes the *invocation line* a host README should print, and means a README for a non-slash-command host must keep a one-line manual fallback: "or open the installed `remindb-setup` SKILL.md and follow it by hand."

The `user-invocable: true` in the wizard frontmatter guarantees the Claude Code / OpenClaw exposure rather than relying on each host's implicit default.

## Env-wiring principle — scoped over global

Two ways to set `REMINDB_DB` / `REMINDB_SOURCE`:

- **PRIMARY — scoped edit** of the loaded config the running server reads (named per host in the matrix above). The env scopes to the remindb subprocess only — multiple remindb workspaces/brains on the same machine stay isolated.
- **FALLBACK — global shell export** (`export REMINDB_DB=… REMINDB_SOURCE=…` in the shell rc). Survives plugin updates and is picked up via `${VAR}` passthroughs in `.mcp.json`-style files, **but a global export overrides every other remindb session launched from that shell** — forces them all to one DB/source. Unsafe when multiple brains coexist; use only when scoped isn't viable.

The wizard prefers the scoped edit on every host. It falls back to a shell-export snippet only when:

1. **Unknown / undetected host** — no known scoped target. Emit a generic `mcpServers` `env` snippet (the shape in the top-level README) and tell the user where their host reads MCP config.
2. **Claude Code marketplace — user opt-in.** The scoped target (cache `.mcp.json`) is editable, but regenerated on every plugin update. The wizard writes it by default and warns the edit is lost on update; it emits the shell-export snippet only if the user explicitly prefers the env to survive updates and accepts the multi-session clobber. Local-checkout installs (`--plugin-dir`) own the file — scoped edit is durable, no fallback needed.
