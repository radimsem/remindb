# MCP resources — passive observation, never a tool

> A tool is the agent reaching for memory. A resource is something watching the memory from outside. They must not be confused, because one of them warms what it touches and the other must not.

[← back to README](../README.md) · related: [architecture](./architecture.md) · [temperature](./temperature.md) · [search](./search.md)

## Why resources exist at all

The `Memory*` tools are the agent's hands — it calls `MemorySearch`, `MemoryFetch`, `MemoryTree` to *use* memory, and every such read is a signal: the agent cared about these nodes, so they warm up (temperature boost) and rank higher next time.

A resource is for a different consumer: a desktop client, a dashboard, anything **rendering** the database rather than reasoning over it. That consumer needs to read state without *being* a signal. If a UI that draws the temperature heatmap boosted every node it displayed, the heatmap would measure its own rendering instead of the agent's attention. So resources are defined by what they must **never** do.

## The resource contract

A resource read is passive observation. It:

- **never boosts temperature** — observing the graph is not attending to it (this is the whole reason resources are separate from `Memory*` read tools, which always boost via `boostResultNodes`);
- **never takes `Store.OpMu`** — it is a pure read, it must not serialize against writers;
- **never emits a snapshot** — there is no state change to record.

This is the inverse of the read-tool discipline in [`.claude/rules/mcp-tool-conventions.md`](../.claude/rules/mcp-tool-conventions.md) §5–6: read *tools* boost and a write *tool* locks; a *resource* does neither, by construction. The separation is structural — resources live in `pkg/mcp/resources/` with their own `Deps` that has no `Tracker` and no emitter, so the no-boost/no-lock/no-snapshot invariant can't be violated by forgetting a convention.

## URI scheme

Resources are addressed under the `remindb://` scheme. Most are **static** (a fixed URI, no parameters); one is **templated** (a URI pattern with a variable):

```
remindb://overview              →   application/json   (static)
remindb://files                 →   application/json   (static)
remindb://tree                  →   application/json   (static — full hierarchy)
remindb://tree/{rootId}{?depth} →   application/json   (templated — bounded subtree)
```

Static resources answer "what is the state of the whole database". The templated `remindb://tree/{rootId}{?depth}` (registered via `AddResourceTemplate`, RFC 6570 form so the matcher spans the optional `?depth=N` query) answers "the subtree under this node". The static/templated split is a deliberately separate mechanism mirroring the resources/tools split: predictable surface first, parameterised surface only where a concrete need appeared.

## The `overview` envelope

`remindb://overview` exposes the same introspection `MemoryStats` reports, but as stable JSON instead of formatted text. Both are pure projections of `inspect.Collect()` — one source of truth, two presentations, zero duplicate stat logic.

```json
{
  "db_path": "...", "db_bytes": 0,
  "nodes":       { "total": 0, "by_type": { "heading": 0 }, "tokens": 0 },
  "snapshots":   { "count": 0, "head_id": 0, "cursor_hash": "", "latest_message": "", "latest_age_s": 0 },
  "temperature": { "avg": 0.0, "median": 0.0, "hot": 0, "cold": 0, "pinned": 0 },
  "relations":   { "total": 0, "by_origin": { "parsed": 0, "manual": 0 }, "pending": 0 },
  "fts_rows": 0
}
```

The shape is **locked** — clients depend on these keys. Notes on two fields that could be read two ways:

- `snapshots` is all zero-valued when the database has no snapshots yet (`head_id: 0`, `cursor_hash: ""`); it never omits keys.
- `relations.total` is the sum of `by_origin` (parsed + manual edges). `pending` is reported as its own sibling and is **not** folded into `total` — the envelope keeps resolved and pending relations distinct so a client can show both.

## The `files` envelope

`remindb://files` exposes the compiled source files grouped by compile root, with per-file node and token counts — the JSON twin of `remindb inspect --files`. Both project the same `store.ListFileSummaries()` query: one source of truth, two presentations, zero duplicate query logic.

```json
{
  "roots": [
    {
      "root": "/repo/docs",
      "files": [
        { "path": "docs/architecture.md", "nodes": 12, "tokens": 840 }
      ]
    }
  ]
}
```

The shape is **locked** — clients depend on these keys. Notes:

- `roots` is sorted by compile root; the empty-string root (files not attributed to a compile root — "ungrouped") always sorts **last**. The envelope reports the bare `""` root, not a `(ungrouped)` label — the client owns the display string.
- Within each root, file order is whatever `ListFileSummaries` returns; only the cross-root grouping is added here.
- Empty database → `{ "roots": [] }`; the key is never omitted.

## The `tree` envelope

`remindb://tree` exposes the parent/child node hierarchy as nested JSON — the structured twin of the `MemoryTree` tool's indented text. Both walk the same `store.GetAllNodes()` + `store.BuildTree()` primitives: one source of truth, two presentations. The text form is for the agent reasoning in prose; the JSON form is for a UI drawing the tree.

The templated `remindb://tree/{rootId}{?depth}` returns only the subtree rooted at `rootId`. `?depth=N` bounds it to `N` descendant generations below that root; omitting the query (or `depth=0`) returns the full subtree.

```json
{
  "roots": [
    {
      "id": "aB3", "type": "heading", "label": "Architecture", "depth": 0,
      "tokens": 120, "temperature": 0.42, "source": "docs/architecture.md",
      "children": [
        {
          "id": "aB3-1", "type": "text", "label": "Overview", "depth": 1,
          "tokens": 80, "temperature": 0.31, "source": "docs/architecture.md",
          "children": []
        }
      ]
    }
  ]
}
```

The shape is **locked** — clients depend on these keys. Notes:

- `roots` is always present: an empty database → `{ "roots": [] }`; the static read returns the whole forest, the templated read returns exactly one element (the requested root).
- Every node carries all eight keys including `children` (`[]` at a leaf, never omitted). `source` is the node's source path made relative to the latest compile root, matching `MemoryTree`; `depth` is the node's own depth in the full tree, unaffected by `?depth` bounding.
- An unknown `rootId` is an **error**, not an empty body — the client gets a failed read, distinguishing "no such node" from "node with no children".

## What stays out

This is read-only state introspection. Resources never mutate, never accept arguments that change the database, and are not a second way to do what a tool does. If a consumer needs to *change* memory, that is a `Memory*` write tool, with its snapshot and its lock — not a resource.
