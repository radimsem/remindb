# Knowledge graph — lateral links between notes

> A tree captures hierarchy. It can't capture "this is related to that" across branches. Memory needs both.

[← back to README](../README.md) · related: [node tree](./node-tree.md) · [versioning](./versioning.md)

## The problem

The [node tree](./node-tree.md) is great at structure — parent, child, section, subsection. What it can't express is a sideways connection: this deployment note matters to that architecture decision, even though they live in different files under different headings. In raw Markdown you'd just write prose and hope the agent notices. It usually doesn't.

## Authored edges

Write a wiki-link in any Markdown or HTML payload and the compiler turns it into a real directed, weighted edge:

```
[[Architecture; w=2.5]]
```

or, in HTML:

```html
<knowledge weight="2.5">Architecture</knowledge>
```

The compiler resolves `Architecture` to the target heading and records `source → target` with a weight. **Higher weight means a more important connection** — it ranks `MemoryRelated` output and acts as a `weight_min` filter.

If the target doesn't exist yet (forward reference, a typo, a heading not compiled yet), the edge is kept **pending** with the unresolved hint and retried on every subsequent compile. Write a heading later and the dangling link self-heals — no manual cleanup.

## Traversing

`MemoryRelated` walks the graph from an anchor:

```
MemoryRelated(anchor="<id>", direction="out", depth=1)
MemoryRelated(anchor="<id>", direction="both", depth=2, weight_min=1.5)
```

- `direction` — `out` (forward), `in` (backlinks, Obsidian-style — edges are one-way), or `both`.
- `depth` — 1–5 hops; higher surfaces transitive connections.
- `weight_min` — drop edges below this importance.

Results rank by **summed path weight**: each hop's weight adds up and the heaviest path to each target wins. A direct `w=2.5` edge beats a `1+1` two-hop chain; a `1.5+2.0` chain (path weight 3.5) beats both. Surfaced targets get a temperature boost, same as a [search](./search.md) hit.

## Manual edges

Sometimes two notes are related and nobody wrote a `[[Label]]`. `MemoryRelate` creates the edge directly, resolving the target the same way parsed links do (id → source+label → label only). Both origins can coexist for the same pair — `parsed` and `manual` are distinct rows.

## One deliberate exception to the version trail

Relations are a **sideband**. `MemoryRelate` does *not* create a snapshot, and the graph does *not* appear in `MemoryDelta` or `MemoryHistory` — the [version trail](./versioning.md) tracks node content only. To see the current state of the graph, call `MemoryRelated`. This is intentional: edges change far more often than content, and folding every relation tweak into the diff trail would bury the changes that actually matter. When a target node is deleted its incoming edges fall back to pending, and re-resolve if a same-label heading reappears.
