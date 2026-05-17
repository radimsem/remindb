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

Resources are addressed under the `remindb://` scheme. The first one is **static** (a fixed URI, no parameters):

```
remindb://overview   →   application/json
```

Static resources answer "what is the state of the whole database". Future per-node or per-snapshot views, if added, would be **templated** resources (`remindb://node/{id}`) registered via `AddResourceTemplate` — a deliberately separate mechanism so the static/templated split mirrors the resources/tools split: predictable surface first, parameterised surface only when a concrete need appears.

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

## What stays out

This is read-only state introspection. Resources never mutate, never accept arguments that change the database, and are not a second way to do what a tool does. If a consumer needs to *change* memory, that is a `Memory*` write tool, with its snapshot and its lock — not a resource.
