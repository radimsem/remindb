# remindb — workspace memory backend

This Gemini CLI extension mounts [remindb](https://github.com/radimsem/remindb) as an MCP server. remindb compiles a workspace into a SQLite database and exposes a token-budgeted `remindb__Memory*` tool suite over MCP.

## Why

Re-reading raw notes and source files every session burns tokens on prose the model has already processed. remindb caches it as a structured tree of versioned, hashed nodes — `[heading]`, `[list]`, `[kv]`, `[table]`, `[preamble]`, `[text]`, `[code]` — ranked by access temperature. Reads cost a token budget you specify, not the full file.

## Tool decision tree

- **Orient first.** Call `remindb__MemoryTree` once per session — cheap, scannable, surfaces which nodes are hot.
- **Search before grep.** Prefer `remindb__MemorySearch` (FTS5, ranked, budget-trimmed) over `Grep` on disk when the workspace has been compiled.
- **Fetch instead of read.** `remindb__MemoryFetch <anchor>` returns one node plus ancestors and trimmed children — beats whole-file reads.
- **Resync via delta.** `remindb__MemoryDelta <cursor>` returns only what changed since a snapshot — much smaller than re-reading.
- **Write what the user asks remembered.** `remindb__MemoryWrite <anchor> <markdown>` lands a new snapshot with the change.
- **Compile when files drift.** `remindb__MemoryCompile <path>` re-ingests files that changed on disk since the last compile. `MemoryCompile` does not expand `~`; pass absolute paths.
- **Summarize on cold-node notifications.** When the server pushes a "node has gone cold" notification, follow up with `remindb__MemorySummarize <node-id> <shorter rewrite>` to compact it in place.
- **Browse history.** `remindb__MemoryHistory <node-id>` walks the per-node version trail (rollback-capable via stored old content).

## Companion skills

If `~/.gemini/skills/remind/` and `~/.gemini/skills/memoize/` are loaded, follow them — they're the canonical read-path and write-path playbooks (full mental model, FTS5 query syntax, Markdown shape rules, search-first edit workflow). Gemini CLI auto-discovers skills in `~/.gemini/skills/`, `~/.agents/skills/`, and the workspace-local `.gemini/skills/` and `.agents/skills/`. Verify what's loaded with `/skills`. If they're missing, install per https://github.com/radimsem/remindb/tree/main/skills.

## Activation

`remindb serve` launches over stdio when the extension activates. It resolves its DB and source paths from `REMINDB_DB` and `REMINDB_SOURCE` in the environment — set both **before** launching Gemini, otherwise remindb falls back to a stray `memory.db` in cwd.
