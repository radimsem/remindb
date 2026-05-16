# How it's put together

> Two phases, one SQLite file in between.

[← back to README](../README.md) · related: [node tree](./node-tree.md) · [versioning](./versioning.md) · [search](./search.md) · [temperature](./temperature.md)

<p align="center">
  <img src="../assets/arch.svg" alt="remindb architecture — compiler phase, SQLite, MCP runtime" width="100%" />
</p>

The shape of remindb is deliberately boring: a compiler turns source files into versioned nodes at ingest time, and an MCP runtime answers the agent in milliseconds on every call. The `.db` is the entire handoff between them — copy it, commit it, sync it, and any MCP-capable agent picks up exactly where the last one left off. No daemon, no external state, nothing to stand up.

Here's what each layer is responsible for:

| Layer | Responsibility |
|-------|----------------|
| **Parser** | One dispatcher, format-specific stages for Markdown, HTML, YAML, JSON/JSONL, TOON. Emits a unified `[]*ContextNode` tree with `id`, `parent_id`, `label`, `content`, `node_type`, `depth`, `token_count`, `content_hash`. |
| **Transformer** | Generates 11-char base62 IDs (xxhash64), estimates cl100k-base tokens, compresses whitespace, decides plain vs. TOON per node. |
| **Diff Engine** | Compares the fresh AST against the last snapshot, produces `add`/`mod`/`rem` deltas, hashes the full state into a new `cursor_hash`. |
| **Emitter** | Writes nodes, diffs, and the new snapshot in one transaction; maintains the FTS5 index via triggers. |
| **Store** | SQLite with WAL mode. Tables: `nodes`, `snapshots`, `diffs`, `cursors`, `relations`, `pending_relations`, plus the `nodes_fts` virtual table. |
| **Query Engine** | Token-budgeted context assembly. Walks ancestors and descendants via `parent_id`, ranks by relevance weighted by temperature, formats output. |
| **Temperature** | Boosts on read, decays on a tick. Cold nodes get flagged for summarization. |
| **MCP Server** | `modelcontextprotocol/go-sdk` over stdio or streamable HTTP. Registers the `Memory*` tool suite, dispatches to the query engine, and notifies clients when nodes go cold. |
| **Rescan Loop** | Optional background goroutine that polls the source directory and triggers incremental recompilation without bringing the server down. |

The pipeline is `parser → transformer → emitter → store` on the way in, and `query → mcp/tools` on the way out. Each layer has its own deep-dive: the tree the parser and transformer produce is [the node tree](./node-tree.md); the diff engine and emitter back [versioning](./versioning.md); the query engine is [search](./search.md); the [temperature](./temperature.md) tracker is the only thing that mutates state in the background.

If you want the *why* behind any one piece, those four pages are where the reasoning lives — this page is just the map.
