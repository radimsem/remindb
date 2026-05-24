# Wiki-links and manual relations

Reference for `memorize`. Load when authoring `[[Label]]` cross-references in a **compiled file** or connecting two existing nodes with `MemoryRelate`.

## Authoring wiki-links — graph relations in compiled content

A `[[Label]]` becomes a **parsed edge** only on the **compile plane** — in a file under `$REMINDB_SOURCE` that the parser processes (see `references/write-paths.md`). It does **not** resolve in a `MemoryWrite` payload (that never parses — the `[[Label]]` would sit as literal text in one flat node). On compile, the parser strips resolver params from the stored content and captures them as edge metadata; readers see the clean normalized `[[Label]]`.

```
[[Architecture]]                                # bare label
[[Architecture; w=2.76]]                         # weight
[[Architecture; w=2.76; source=docs/ARCH.md]]    # source-qualified
[[Architecture; w=2.76; id=3kGXxidmWBp]]         # explicit target ID
[[3kGXxidmWBp]]                                  # bare ID (11 base62 chars)
```

HTML alternative (same normalized result): `<knowledge>Architecture</knowledge>`, `<knowledge weight="2.76" source="docs/ARCH.md">Architecture</knowledge>`, etc. Params (`w`, `source`, `id`) live in the source file on disk — re-extracted every `MemoryCompile`.

**Resolution priority** (exact order):

1. `id=<hint>` → lookup by ID. **No fallback** if missing — edge goes pending.
2. `source=<file>` + label → label match restricted to that file. **No fallback** on miss — pending. `source=docs/x.md` matches the exact stored path and absolute paths ending `/docs/x.md`.
3. Label only → match across all heading nodes, case-insensitive, whitespace-trimmed. First wins by `(source_file ASC, depth ASC, id ASC)`.

The hard-constraint for id/source is intentional — a typo yields a discoverable pending row instead of silently linking the wrong heading; fix it and the next compile self-heals.

**Weight** (`REAL`, default `1.0`, higher = more important): `1.0` regular · `2.0`–`5.0` emphasized (surface first) · `< 1.0` weak. Ranks `MemoryRelated` by sum-along-path; `weight_min` filters. `[[X; w=3]]` is a deliberate "this matters" signal — don't waste it.

`[[X]]` inside fenced code blocks or `<code>` is example text — left verbatim, no edge. Intentional: documentation syntax must not generate relations.

## MemoryRelate — manual edges between existing nodes

For connecting two nodes with no `[[Label]]` in their source — typically after a conversation reveals two memories are related.

```
remindb__MemoryRelate(source_id="<id>", target_label="Architecture")
remindb__MemoryRelate(source_id="<id>", target_label="Architecture", target_source="docs/ARCH.md", weight=2.5)
remindb__MemoryRelate(source_id="<id>", target_id="3kGXxidmWBp", weight=2.0)
```

Same resolution priority as parsed links (id → source+label → label only; narrowing modifiers are hard constraints). Hit → resolved edge; miss → pending, retried every compile.

**Snapshot-free** — edges don't appear in `MemoryDelta`/`MemoryHistory`, cursor doesn't move; inspect via `MemoryRelated`. (Edges churn faster than content; binding them to the cursor would pollute the diff trail.) **Prefer `target_label` + `target_source` over `target_id`:** structural IDs are content-addressed on `(source_file, parent_id, sibling_index)` — a sibling reorder rotates later IDs and a `target_id` edge silently mispoints or orphans. Label+source re-resolves on the next compile. `parsed` and `manual` origins are independent (`UNIQUE(source_node_id, target_node_id, origin)` allows both); `MemoryRelated` dedups by target node.
