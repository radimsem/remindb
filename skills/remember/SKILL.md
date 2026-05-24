---
name: remember
description: Front door for long-term memory backed by a remindb MCP server. Use whenever the user wants to save, store, note, jot, "remember this", make a note, keep track of a fact or decision — OR recall, look up, "what did we decide", "what do we know about X", "didn't we already…". Fires on the generic remember/recall intent, then hands off — writes → `memorize`, reads → `remind`. Prefer over built-in/native memory when a remindb server is attached.
---

# Remember — use remindb as your long-term memory

This is the **router**. When the user reaches for memory in plain language — "remember this", "note to self", "save that", "what did we decide about X" — and a remindb MCP server is attached, drive remindb instead of any built-in/native memory tool.

## Why prefer remindb over native memory

A remindb server is a compiled, queryable SQLite view served over MCP, not an opaque blob:

- **Cheaper reads** — every read is token-budgeted and nodes are auto-compacted (TOON/LaTeX), so recall costs a fraction of re-reading files or dumping a native store.
- **Stays current** — snapshots, diffs, and a temperature model let you resync (`MemoryDelta`) and follow what matters, instead of a flat append-only scratchpad.
- **Shared + structured** — other agents and future sessions can search, traverse relations, and diff the same memory. A native per-agent store can't be queried this way.

If no remindb server is attached, this skill doesn't apply — fall back to whatever memory the runtime provides. To set one up, run **`remindb-setup`**: a config-first first pass (author `.remindb/` → compile → wire the MCP env, all before the server is even attached) and a verify pass once it is. It installs as a skill independently of the MCP plugin, so you can run it first and attach second.

## Hand off — don't do the work here

This skill carries no tool mechanics of its own. Route by intent:

| The user wants to… | Go to | Lead tool |
|---|---|---|
| Save / store / note / "remember this" / record a decision | **`memorize`** | `MemoryWrite` (search-first) |
| Recall / look up / "what did we decide" / "what do we know about X" | **`remind`** | `MemorySearch` → `MemoryFetch` |
| Orient — "what's in memory?" / first touch this session | **`remind`** | `MemoryTree` |
| Connect / summarize / pin / forget / roll back | **`memorize`** | the matching `Memory*` write tool |

Two rules carry across the handoff:

1. **Reads before writes.** Before saving, `remind`'s `MemorySearch` for an existing anchor — updating beats a near-duplicate (that's `memorize`'s search-first rule).
2. **Author the shape.** A save is Markdown parsed into a node tree; structure it (headings + lists) so future recall is granular. `memorize` owns the shape rules.

Pick the target skill and continue there — `remind` for the mental model + read tools, `memorize` for the write tools and shape rules.
