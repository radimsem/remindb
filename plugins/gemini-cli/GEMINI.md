# remindb — workspace memory backend

This Gemini CLI extension mounts [remindb](https://github.com/radimsem/remindb) as an MCP server. remindb compiles a workspace into a SQLite database and exposes the full `remindb__Memory*` tool suite over MCP.

Prefer `MemorySearch` / `MemoryFetch` over re-reading files when the content has already been compiled. Use `MemoryWrite` to persist notes the user asks to remember; use `MemoryCompile` when files on disk have drifted from the compiled view.

`remindb serve` is launched over stdio when the extension activates. It resolves its database and source paths from `REMINDB_DB` and `REMINDB_SOURCE` in the environment.
