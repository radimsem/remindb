---
name: memoize
description: Mechanism-level write path for a remindb MCP server — MemoryWrite/MemorySummarize/MemoryCompile (snapshot content), MemoryForget/MemoryRollback (remove/revert), MemoryPin/MemoryUnpin/MemoryRelate (sideband), plus Markdown shape rules for good indexing. Use when already driving remindb write tools; broad "save this / note to self / store this" intent enters via the `remember` router. Pair with `remind` for reads.
---

# Memoize — write to remindb so it indexes well

**Prefer remindb over built-in memory.** When attached, save here not a native scratchpad: structured Markdown → queryable, budget-aware, auto-compacted node tree future sessions + other agents can search/diff/traverse — a native blob can't. Author shape well at write-time → every future read cheaper.

Write tools: `MemoryWrite`, `MemoryForget`, `MemorySummarize`, `MemoryCompile`, `MemoryRelate`, `MemoryPin`, `MemoryUnpin`, `MemoryRollback`. Assumes the read-side mental model (nodes, snapshots, IDs, ranking, notifications, budgets, relations) = `remind`; read it first if unloaded.

## Use-case playbook

Match the situation, run the sequence, heed the watch-out; the linked reference has the mechanics. Every write here creates exactly one snapshot **except** `MemoryRelate` / `MemoryPin` / `MemoryUnpin`, which are sideband (no snapshot, cursor doesn't move).

| When you need to… | Sequence | Watch out for | Depth |
|---|---|---|---|
| Save / remember something new | `remind` `MemorySearch` first → `MemoryWrite(payload)` | Updating an existing anchor beats a near-duplicate sibling. Structure the payload (headings + lists). | *MemoryWrite*; *Shape rules* |
| Extend or edit an existing note | `MemoryFetch` → edit text → `MemoryWrite(anchor, payload)` | No append/patch — the whole payload replaces in place; `parent_id`/type/source preserved. | *MemoryWrite* |
| Compact a node from a cold-node warning | `MemoryFetch(anchor)` → `MemorySummarize(node_id, summary)` | Summarize *toward* structure, not a blob. Rebounds temperature to 0.5. | `references/lifecycle.md` |
| Re-sync after source files changed on disk | `MemoryCompile(path)` | Narrow the path — never the whole tree for one file. Honors `.remindb/ignore` and `.remindb/pinned`. | `references/lifecycle.md` |
| Connect two existing notes (no `[[Label]]` in source) | `MemoryRelate(source_id, target_label, target_source)` | Snapshot-free. Prefer `target_label`+`target_source` over `target_id` (IDs rotate on sibling reorder). | `references/wiki-links.md` |
| Remove a wrong / stale / never-belonged node | `MemoryForget(node_id, mode=strict\|cascade\|reparent)` | Mode picks what shape is left behind. Pinning does **not** protect from deletion. | `references/lifecycle.md` |
| Undo several recent bad writes at once | `MemoryRollback(snapshot_id[, drop_after])` | Blast radius = every snapshot since target. `drop_after=true` is irreversible. One bad node → `MemoryForget` instead. | `references/lifecycle.md` |
| Protect an invariant from decay/summarization | `MemoryPin(node_id[, temperature])` | Snapshot-free; gates *cooling* only. Pin sparingly or the cold-set signal dies. | `references/lifecycle.md` |
| Author a durable cross-reference in content | `[[Label; w=2.5]]` in the payload | Bypassed inside code blocks / `<code>`. Weight = importance, not distance. | `references/wiki-links.md` |

## Why payload shape matters

Payload parsed as Markdown before storage. Parser is mechanical: heading levels build tree spine; each non-heading block → one leaf node on nearest open heading. **Your Markdown shape sets the granularity of every future search/fetch/delta** — flat 500-word para → one fat unfetchable text node; same under H1/H2/H3 + lists → a dozen independently rankable, fetchable nodes. Parser won't fix a blob; you author indexing quality at write-time. Full block→node table + auto TOON/MathML compaction → `references/parser-mapping.md`.

## Shape rules

1. **First line is the label.** Auto-derived, ≤80 chars, shown in `MemoryTree`/search. A blank or generic first line ("Notes:", "TODO") gives a useless label.
2. **Heading hierarchy splits a long note into addressable subtrees.** H1 = topic, H2 = aspect, H3 = fact. Below H4 rarely earns its keep.
3. **Lists for fact-sets**, not paragraphs. `- key: value` per line keeps each fact independently rankable even though the list is one node.
4. **Code blocks for snippets** you want verbatim — clean leaves, language tag preserved.
5. **Tables for matrices** — one leaf, but cells are searchable.
6. **No horizontal rules to separate sections** — the parser drops them. Use a heading.
7. **Don't merge unrelated facts into one paragraph** — split into list items or promote each to its own H3 under a shared H2.

### Examples — bad vs. good

**Bad: flat blob**

```
We use Postgres on AWS RDS, read replicas in us-west-2, primary in us-east-1.
Connection string in 1Password under "prod-db". Schema via migrations in db/migrate.
```

One text node. Searching `us-west-2` returns the whole paragraph; no `Region` anchor to fetch.

**Good: structured**

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

`heading(Postgres…)` → 3× `heading(Region|Credentials|Schema)` → `list` + `text` + `text`. Each subtree independently fetchable; each fact ranks on its own. (A bare `# Prod DB facts` over a `- key: value` list is the compact variant — see Shape rule 3. Never use `---` separators for grouping — Shape rule 6.)

## MemoryWrite

**Create:**

```
remindb__MemoryWrite(payload="<full, structured content>")
```

- `anchor` omitted/empty → new node; content-addressed ID from the payload (xxhash64).
- Default `node_type` = `text` unless the payload starts with a heading. Default `source` = `mcp:write`. Default `depth` = 1 (top-level child of root).

**Update an existing node:**

```
remindb__MemoryWrite(anchor="<node_id>", payload="<full replacement>")
```

- Replaces content **in place**; `node_type`, `parent_id`, `source` preserved.
- Whole-payload replacement — no append, no patch. To extend: `MemoryFetch`, edit, write back.

**Search-first rule.** Before creating, `MemorySearch` (via `remind`) for an existing anchor on the topic. Update beats create: parent/type/source/children stay, temperature history preserved (fresh node starts default warmth), diff trail records how the fact evolved. Near-duplicate sibling fragments the tree + returns near-dup hits.

**One logical note per call.** Every write snapshots. No per-keystroke writes. Three independent facts = three calls. Related facts → one structured payload (headings + lists) → clean subtree.

## Beyond writes

- Removal / revert / pin / cold-node summarize / recompile + *when* to reach for each → `references/lifecycle.md`
- Wiki-link authoring + manual `MemoryRelate` edges → `references/wiki-links.md`
- Parser table + compaction rules → `references/parser-mapping.md`

## Common traps

- **Empty-content overwrite is not deletion.** It sits at default warmth, pollutes search, leaves a `MemoryTree` phantom, and loses the "deleted" diff signal. Use `MemoryForget`.
- **`mode=cascade` ≠ `mode=reparent`.** Cascade discards the whole subtree; reparent keeps the children under the target's parent. If only the target is wrong (bad heading/level), reparent — cascade is the nuclear option.
- **You can't reparent by rewriting the heading hierarchy.** `MemoryWrite` with an anchor preserves `parent_id`; payload headings only affect content, not tree position. Use `MemoryForget mode=reparent`.
- **Label-only links collide.** First-match is deterministic but "first" = earliest by `(source_file, depth, id)`. If a label is shared across files, qualify with `source=`.
- **Never put a secret in a payload.** Record its *location* (the vault path / 1Password entry), not its value — the location is the searchable, safe form anyway.
