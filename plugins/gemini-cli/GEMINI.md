# remindb — workspace memory backend

This Gemini CLI extension mounts [remindb](https://github.com/radimsem/remindb) as an MCP server. remindb compiles a workspace into a SQLite database and exposes eight token-budgeted tools over MCP:

- `remindb__MemoryTree` — structural overview of the compiled workspace
- `remindb__MemorySearch` — full-text search within a token budget
- `remindb__MemoryFetch` — context around an anchor node within a token budget
- `remindb__MemoryWrite` — write or update content at an anchor, creating a snapshot
- `remindb__MemoryDelta` — changes since a given snapshot
- `remindb__MemorySummarize` — replace a node's content with a provided summary
- `remindb__MemoryHistory` — browse version history for a node
- `remindb__MemoryCompile` — re-compile source files or a directory

Prefer `MemorySearch` / `MemoryFetch` over re-reading files when the content has already been compiled. Use `MemoryWrite` to persist notes the user asks to remember; use `MemoryCompile` when files on disk have drifted from the compiled view.

`remindb serve` is launched over stdio when the extension activates. It resolves its database and source paths from `REMINDB_DB` and `REMINDB_SOURCE` in the environment.
