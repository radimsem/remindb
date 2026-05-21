# Relations — the graph layer

Reference for `remind`'s `MemoryRelated`. Load when you follow a `[[Label]]` cross-reference or need traversal depth/weight semantics.

## The model

Directed weighted edges beyond the parent/child tree. Two kinds:

- **Resolved** — `source → target` between real node IDs. From the parser (`[[Label]]` wiki-link in source) or `MemoryRelate` (a `memoize` tool).
- **Pending** — target unresolved at compile time (forward ref, typo, not yet compiled). Stored with the hint, retried every compile.

Fields: `weight` (`REAL`, default `1.0`; higher = more important; ranks `MemoryRelated`, filters via `weight_min`; not yet in `MemorySearch` ranking). `origin` = `parsed` or `manual` (both can coexist per pair; `UNIQUE(source, target, origin)`). Direction is one-way (Obsidian-style); backlinks via `direction=in`.

**Relations are a sideband** — they never appear in `MemoryDelta`/`MemoryHistory`, and `MemoryRelate` doesn't snapshot. Inspect the graph only via `MemoryRelated`. When a target is deleted, incoming edges go resolved→pending (label preserved); a same-label heading reappearing self-heals them on the next compile.

## Traverse: MemoryRelated

A `[[Label]]` marker in fetched content is an authored cross-reference. Surface the linked context:

```
remindb__MemoryRelated(anchor="<node_id>", direction="out", depth=1)
remindb__MemoryRelated(anchor="<node_id>", direction="both", depth=2, weight_min=1.5)
```

- `direction` — `out` (forward), `in` (backlinks), `both` (default).
- `depth` — hops 1–5 (default 1).
- `weight_min` — drop edges below this (default 0); `1.0` ignores weak links.
- `budget` — response token cap (default 1000).

Ranks by **summed path weight** (heaviest path to each target wins), then temperature. Direct `w=2.5` beats `1+1`; `1.5+2.0` (3.5) beats both. Each row shows `hop=N` and `weight=N.N`. Surfaced targets get a temperature boost (like `MemorySearch`).
