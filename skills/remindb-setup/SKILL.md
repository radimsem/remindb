---
name: remindb-setup
description: Connect + configure the remindb "brain" from inside an agent session — confirm the MCP server is attached and bound to the right DB, author the `.remindb/` workspace config (ignore/pinned/temperatures/config.json), run the one-shot SetLoggingLevel handshake so cold-node notifications arrive. Use when remindb tools are missing/misconfigured, on "no results"/wrong-workspace symptoms, or first-time workspace memory setup. Runtime guidance — not a substitute for the per-agent plugin install README.
---

# remindb-setup — connect and configure the brain

Agent-runtime guidance for getting an *already-installed* remindb MCP server working + authoring the workspace config it reads. **Does not duplicate plugin install docs** — installing the binary + registering the MCP server lives in the per-agent plugin README (`plugins/<agent>/README.md`). Tools absent entirely → start there; tools should-be-present → here.

## 1. Verify the server is attached and bound

The tools surface under your agent's MCP namespace (e.g. `remindb__MemoryStats`). If they're absent, the server isn't connected — fix that in the plugin README, then return. If they're present, confirm it's pointed at the right workspace:

```
remindb__MemoryStats()
```

The plain-text block leads with the **DB path** and node count. Check both:

- **Wrong / stray DB path** (e.g. a `memory.db` in cwd) → `REMINDB_DB`/`REMINDB_SOURCE` weren't exported before the agent launched, so the server fell back. Set them and relaunch (plugin README §"Point remindb at your workspace").
- **Zero nodes** → nothing has been compiled into this DB yet. The DB is built from a source tree before reads work; compile a source dir (plugin README §"Compile a source directory") or, for in-session seeding, `remindb__MemoryCompile(path="<abs path>")` with an absolute path (`~` is not expanded).

`remindb://doctor` (or `remindb doctor` on the CLI) is the deeper check — it flags FTS5 desync and stale compile roots.

## 2. The SetLoggingLevel handshake — turn on cold-node notifications

Cold-node nudges (`level: "warning"`, `logger: "remindb.temperature"`) are pushed only to client sessions that have set a logging level. A session that never calls `SetLoggingLevel` is silent — temperature decay still happens server-side, but you never get told which nodes went cold, so the `MemorySummarize` handoff never triggers.

This is a **one-shot MCP request per session**, sent over the protocol — not a tool call you can issue from the model turn. Where it lives depends on the runtime:

- Most MCP clients send `logging/setLevel` automatically (or expose a setting). If your client does, notifications arrive with no action.
- If yours doesn't, it's a client-side capability gap, not a remindb misconfiguration — the server is ready; the client must opt in. Surface it to the user as "your MCP client needs to call `logging/setLevel` (e.g. `debug`) to receive remindb cold-node notifications."

A useful confirmation: notifications never arrive *before* the handshake. If decay is running (`temperature.enabled` not `false`) and you still see no `remindb.temperature` warnings over time, the handshake is the first thing to suspect.

## 3. Author the `.remindb/` workspace config

remindb reads workspace-level config from a `.remindb/` directory at the **source root** (the dir you compiled / pointed `REMINDB_SOURCE` at). All files are optional; missing means defaults. The directory itself is skipped during source walks, so it never becomes memory nodes. Operators author these once — the agent typically doesn't write them at runtime, but should know what each does to diagnose behavior:

| Entry | What it controls |
|---|---|
| `.remindb/config.json` | Runtime knobs as feature blocks (`budgets`, `temperature`, `server`, …). Unknown keys are **rejected at startup** — a typo fails fast rather than silently no-opping. |
| `.remindb/ignore` | Gitignore-style exclude patterns; subtract from the supported-extension allow-list. Honored by `compile`, the background rescan, and `MemoryCompile` alike. Can't re-include hardcoded skip dirs (`node_modules`, `vendor`, …) or dotfiles. |
| `.remindb/pinned` | Same grammar as `ignore`; pre-seeds the `pinned` column for matched files at insert time, keeping them permanently warm (`README.md`, `**/CONTEXT.md`, security policies). |
| `.remindb/temperatures.json` | Per-path initial-temperature overrides. Composes with `pinned`: `pinned` says *whether* to protect, `temperatures.json` says *at what temperature*. |

`config.json` highlights worth knowing:

- **`budgets`** — per-tool default token budgets (`search`, `fetch`, `fetch_batch`, `related`). Lets a workspace pick sane defaults so callers needn't always pass `budget`.
- **`temperature`** — `enabled` (set `false` to freeze decay + notifications entirely), `cold_threshold` / `hot_threshold` / `notify_threshold`, `tick_interval`. Changing thresholds shifts search ranking *and* the cold-node stream; see `remind` for the model.
- **`server`** — resource debounce windows (`server.resources`), log buffer size, and opt-in per-session logfiles (`server.logging.session_files.enabled`).

The full schema with every key and default is the workspace manual: <https://github.com/radimsem/remindb/blob/main/docs/configuration.md>.

## Once connected

Server attached, DB correct, handshake done? You're ready to use the brain: `remind` for the read tools + mental model, `memoize` for the write tools + shape rules, `remember` as the plain-language front door.
