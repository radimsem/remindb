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

Resources are addressed under the `remindb://` scheme. Most are **static** (a fixed URI, no parameters); some are **templated** (a URI pattern with a variable):

```
remindb://overview                →   application/json   (static)
remindb://files                   →   application/json   (static)
remindb://tree                    →   application/json   (static — full hierarchy)
remindb://tree/{rootId}{?depth}   →   application/json   (templated — bounded subtree)
remindb://graph                   →   application/json   (static — relations graph)
remindb://snapshots               →   application/json   (static — full history)
remindb://snapshots{?limit}       →   application/json   (templated — newest N)
remindb://snapshots/{id}/diffs    →   application/json   (templated — per-snapshot diffs)
```

Static resources answer "what is the state of the whole database". The templated forms (registered via `AddResourceTemplate`, RFC 6570 so the matcher spans the optional query / path variable) answer a narrower question: `remindb://tree/{rootId}{?depth}` the subtree under a node, `remindb://snapshots{?limit}` the newest N snapshots, `remindb://snapshots/{id}/diffs` the diffs of one snapshot. The static/templated split is a deliberately separate mechanism mirroring the resources/tools split: predictable surface first, parameterised surface only where a concrete need appeared.

## The `overview` envelope

`remindb://overview` exposes the same introspection `MemoryStats` reports, but as stable JSON instead of formatted text. Both are pure projections of `inspect.Collect()` — one source of truth, two presentations, zero duplicate stat logic.

```json
{
  "db_path": "/repo/.remindb/memory.db", "db_bytes": 196608,
  "nodes":       { "total": 142, "by_type": { "heading": 38, "text": 96, "code": 8 }, "tokens": 18450 },
  "snapshots":   { "count": 7, "head_id": 7, "cursor_hash": "9f3c1a7e", "latest_message": "write:aB3", "latest_age_s": 42 },
  "temperature": { "avg": 0.37, "median": 0.31, "hot": 12, "cold": 48, "pinned": 3 },
  "relations":   { "total": 27, "by_origin": { "parsed": 22, "manual": 5 }, "pending": 4 },
  "fts_rows": 142
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

## The `graph` envelope

`remindb://graph` exposes the relations knowledge graph — the "brain" view — as stable JSON for a UI that draws it. It is pure exposure: resolved edges come straight from `store.GetAllRelations()`, pending/unresolved edges from `store.GetAllPendingRelations()`, and the node set from `store.GetAllNodes()` — no traversal logic, the same flat queries the rest of the store uses.

```json
{
  "nodes": [
    { "id": "aB3",   "label": "Architecture", "type": "heading", "temperature": 0.42 },
    { "id": "aB3-1", "label": "Overview",     "type": "text",    "temperature": 0.31 }
  ],
  "edges": [
    { "source": "aB3", "target": "aB3-1", "weight": 4.2, "origin": "parsed" },
    { "source": "aB3", "target": "aB3-1", "weight": 1.0, "origin": "manual" }
  ],
  "pending": [
    { "source": "aB3", "target_label": "Roadmap", "target_source": "docs/roadmap.md",
      "target_id_hint": "", "weight": 1.0, "origin": "parsed" }
  ]
}
```

The shape is **locked** — clients depend on these keys. Notes:

- `nodes` is the **referenced set only**: a node appears iff it is the source or target of a resolved edge, or the source of a pending edge. Orphan nodes (no relations) are not in the graph — use `remindb://tree` for the full node hierarchy.
- `edges` are resolved relations (both endpoints exist). `origin` is `parsed` (from the weight wiki-link syntax) or `manual` (from `MemoryRelate`); `weight` defaults to `1.0`. Each edge is directed `source → target`.
- `pending` are unresolved edges: the `source` node exists but the target was never resolved to a node, so it is described by `target_label` / `target_source` / `target_id_hint` (any may be empty) instead of a `target` id. Pending edges are kept a distinct array — never folded into `edges` — so a client can render them differently (dashed, greyed).
- All three keys are always present; an empty database → `{"nodes":[],"edges":[],"pending":[]}`, never `null`.

## The `snapshots` envelope

`remindb://snapshots` exposes the version history behind `MemoryHistory` — every snapshot, newest-first, with the parent links that reconstruct branch topology. It is pure exposure: rows come straight from `store.ListSnapshots`, the HEAD marker from `store.GetHeadSnapshotID` — no new diff logic. `remindb://snapshots{?limit}` returns just the newest `?limit=N` (omit for full history). `remindb://snapshots/{id}/diffs` returns the per-snapshot diff records (`store.GetDiffsBySnapshot`), the data behind `MemoryDelta`.

```json
{
  "snapshots": [
    { "id": 3, "parent_id": 2, "message": "write:aB3", "compile_root": "/repo", "created_at": 1737072000, "is_head": true },
    { "id": 1, "parent_id": null, "message": "compile", "compile_root": "/repo", "created_at": 1737070000, "is_head": false }
  ]
}
```

```json
{
  "snapshot_id": 3,
  "diffs": [
    { "op": "mod", "node_id": "aB3", "old_hash": "h1", "new_hash": "h2", "old_content": "before", "new_content": "after" }
  ]
}
```

The shape is **locked** — clients depend on these keys. Notes:

- Snapshots preserve store order (newest `id` first); the timeline UI reverses for display. `parent_id` is JSON `null` for a root snapshot, never `0` — that distinction is what lets a client draw branches.
- `is_head` marks the snapshot at the cursor. At most one is `true`; if no snapshot has been recorded, none is.
- `snapshots` and `diffs` are always present; an empty database / a snapshot with no diffs → `[]`, never `null`.
- An unparseable `{id}` or non-positive `?limit` is an **error**, not an empty body — the client gets a failed read.

## What stays out

This is read-only state introspection. Resources never mutate, never accept arguments that change the database, and are not a second way to do what a tool does. If a consumer needs to *change* memory, that is a `Memory*` write tool, with its snapshot and its lock — not a resource.
