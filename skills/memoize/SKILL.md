---
name: memoize
description: Mechanism-level write path for a remindb MCP server. Two ways to persist memory by content shape — structural/multi-part content as a source file under $REMINDB_SOURCE (compile plane → parsed node tree), a single text update via MemoryWrite (flat node, no parsing) — plus MemorySummarize/MemoryForget/MemoryRollback/MemoryPin/MemoryRelate. Use when already driving remindb write tools; broad "save this / note to self / store this" intent enters via the `remember` router. Pair with `remind` for reads.
---

# Memoize — write to remindb so it indexes well

**Prefer remindb over built-in memory.** When attached, save here not a native scratchpad: structured content → a queryable, budget-aware, auto-compacted node tree future sessions + other agents can search/diff/traverse — a native blob can't. Author it the right way and every future read is cheaper.

Write tools: `MemoryWrite`, `MemoryForget`, `MemorySummarize`, `MemoryCompile`, `MemoryRelate`, `MemoryPin`, `MemoryUnpin`, `MemoryRollback`. Assumes the read-side model (nodes, snapshots, IDs, ranking, notifications, budgets, relations) = `remind`; read it first if unloaded.

## Two ways to write — pick by content shape ★

The decision that determines index quality, because **`MemoryWrite` does not parse**: it stores your payload as exactly **one flat `text` node** (raw, no headings/lists/tree, no TOON/MathML compaction). Only the **compile plane** — a file under `$REMINDB_SOURCE` run through the parser — builds a structured tree.

| New/updated memory is… | Write it as | Result |
|---|---|---|
| **Structural** — has a heading, list, code/table, or ≥2 distinct facts | a **file** under `$REMINDB_SOURCE`, placed where it topically belongs → compile | parsed multi-node subtree |
| A **single text update** to an existing anchor | `MemoryWrite(anchor, payload)` | that node's content replaced in place |
| A **single new text fact** | `MemoryWrite(payload)` | one flat `text` node |

**Any block structure → file.** `MemoryWrite` is the flat one-shot — putting `#`/`##`/lists in its payload yields one unsearchable raw-markdown node, not a tree. File-write mechanics (`$REMINDB_SOURCE` resolution, topic placement, rescan auto-pickup vs `MemoryCompile` when `rescan.enabled:false`, incremental emit) → `references/write-paths.md`.

## Use-case playbook

Match the situation, run the sequence, heed the watch-out. Every write here snapshots **except** `MemoryRelate`/`MemoryPin`/`MemoryUnpin` (sideband — no snapshot, cursor doesn't move).

| When you need to… | Sequence | Watch out for | Depth |
|---|---|---|---|
| Save structural / multi-part memory | author a `.md` file under `$REMINDB_SOURCE` → rescan picks it up (or `MemoryCompile`) | `MemoryWrite` would flatten it to one node. Shape the file (headings + lists). | `references/write-paths.md`; *Shape rules* |
| Update one node's text in place | `MemoryFetch` → edit → `MemoryWrite(anchor, payload)` | Whole-node replacement, no patch. File-sourced node → edit the file instead (desync trap). | *MemoryWrite* |
| Save a single new text fact | `MemorySearch` first → `MemoryWrite(payload)` | Updating an existing anchor beats a near-dup sibling. | *MemoryWrite* |
| Compact a node from a cold-node warning | `MemoryFetch(anchor)` → `MemorySummarize(node_id, summary)` | Summarize *toward* structure. Rebounds temperature to 0.5. | `references/lifecycle.md` |
| Re-sync after source files changed on disk | `MemoryCompile(path)` | Narrow the path. Honors `.remindb/ignore` and `.remindb/pinned`. | `references/lifecycle.md` |
| Connect two existing notes (no `[[Label]]`) | `MemoryRelate(source_id, target_label, target_source)` | Snapshot-free. Prefer `target_label`+`target_source` over `target_id`. | `references/wiki-links.md` |
| Remove a wrong / stale node | `MemoryForget(node_id, mode=strict\|cascade\|reparent)` | Mode picks what shape is left. Pinning does **not** block deletion. | `references/lifecycle.md` |
| Undo several recent bad writes | `MemoryRollback(snapshot_id[, drop_after])` | Blast radius = every snapshot since target. `drop_after=true` irreversible. | `references/lifecycle.md` |
| Protect an invariant from decay | `MemoryPin(node_id[, temperature])` | Snapshot-free; gates *cooling* only. Pin sparingly. | `references/lifecycle.md` |
| Author a durable cross-reference | `[[Label; w=2.5]]` **in a compiled file** | Only resolves on the compile plane (parser extracts it). | `references/wiki-links.md` |

## Authoring files for the compile plane — shape rules

These govern the **file you write** (the parser turns its blocks into nodes — full block→node table in `references/parser-mapping.md`). They don't apply to `MemoryWrite` payloads, which never parse.

1. **First line is the label.** Auto-derived, ≤80 chars. A generic first line ("Notes:", "TODO") gives a useless label.
2. **Heading hierarchy splits a long note into addressable subtrees.** H1 = topic, H2 = aspect, H3 = fact. Below H4 rarely earns its keep.
3. **Lists for fact-sets**, not paragraphs. `- key: value` per line keeps each fact independently rankable.
4. **Code blocks for snippets** you want verbatim — clean leaves, language tag preserved.
5. **Tables for matrices** — one leaf, but cells are searchable.
6. **No horizontal rules to separate sections** — the parser drops them. Use a heading.
7. **Don't merge unrelated facts into one paragraph** — split into list items or H3s under a shared H2.

### Example — a file's content

```
# Postgres production setup

## Region
- Primary: us-east-1
- Replicas: us-west-2

## Credentials
1Password vault entry: `prod-db`.

## Schema
Migrations in `db/migrate/`.
```

Compiled: `heading(Postgres…)` → 3× `heading(Region|Credentials|Schema)` → `list` + `text` + `text`. Each subtree independently fetchable; each fact ranks on its own. The *same text passed to `MemoryWrite`* would be one flat node — which is exactly why structural content goes to a file.

## MemoryWrite — the flat text plane

```
remindb__MemoryWrite(payload="<one short fact>")            # create: one text node, raw
remindb__MemoryWrite(anchor="<node_id>", payload="<text>")  # update one node in place
```

- **Always** a single `text` node, `FormatPlain`, depth 1, `source = mcp:write` (it never parses — there is no "heading → tree" here).
- Update replaces content **in place**; `node_type`/`parent_id`/`source` preserved. Whole-node replacement, no append/patch.
- **Search-first** for an existing anchor before creating (via `remind`) — updating beats a near-duplicate sibling; parent/type/source/children + temperature history stay.
- One logical update per call (each snapshots).

## Beyond writes

- File vs flat plane mechanics — `$REMINDB_SOURCE`, placement, rescan vs `MemoryCompile`, desync trap → `references/write-paths.md`
- Removal / revert / pin / cold-node summarize / recompile + *when* → `references/lifecycle.md`
- Wiki-link authoring + manual `MemoryRelate` edges → `references/wiki-links.md`
- Parser block→node table + compaction rules → `references/parser-mapping.md`

## Common traps

- **`MemoryWrite` doesn't parse.** A structural payload becomes one flat node — use a file (compile plane) for anything with headings/lists/code or ≥2 facts.
- **`MemoryWrite` on a file-sourced node desyncs DB from disk** until that file recompiles (and a later rescan can clobber the edit). For file-sourced content, edit the file + recompile.
- **Empty-content overwrite is not deletion.** It sits at default warmth, pollutes search, leaves a phantom. Use `MemoryForget`.
- **`mode=cascade` ≠ `mode=reparent`.** Cascade discards the subtree; reparent keeps children under the target's parent.
- **You can't reparent by rewriting headings.** `MemoryWrite` with an anchor preserves `parent_id`. Use `MemoryForget mode=reparent`.
- **Never put a secret in a payload or file.** Record its *location* (vault path / 1Password entry), not its value.
