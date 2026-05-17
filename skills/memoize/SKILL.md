---
name: memoize
description: Write memory to a remindb MCP server with Markdown that indexes well into the node tree. Covers shape rules for good indexing, search-first updates, cold-node summarization, source recompile, wiki-link authoring, manual relation edges, pinning nodes against temperature decay, explicit node removal with three deletion modes, and rolling back the graph to a prior snapshot with optional history pruning. Pair with `remind` for reads.
---

# Memoize — write to remindb so it indexes well

Write path: `MemoryWrite`, `MemoryForget`, `MemorySummarize`, `MemoryCompile`, `MemoryRelate`, `MemoryPin`, `MemoryUnpin`, `MemoryRollback`. Assumes the read-side mental model (nodes, snapshots, IDs, ranking, notifications, budgets, relations) — that's `remind`. If those terms aren't loaded, read `remind` first.

## Use-case playbook

Match the situation, run the sequence, heed the watch-out; the linked section has the mechanics. Every write here creates exactly one snapshot **except** `MemoryRelate` / `MemoryPin` / `MemoryUnpin`, which are sideband (no snapshot, cursor doesn't move).

| When you need to… | Sequence | Watch out for | Section |
|---|---|---|---|
| Save / remember something new | `remind` `MemorySearch` first → `MemoryWrite(payload)` | Updating an existing anchor beats a near-duplicate sibling. Structure the payload (headings + lists). | *MemoryWrite*; *Shape rules* |
| Extend or edit an existing note | `MemoryFetch` → edit text → `MemoryWrite(anchor, payload)` | No append/patch — the whole payload replaces in place; `parent_id`/type/source preserved. | *Update an existing node* |
| Compact a node from a cold-node warning | `MemoryFetch(anchor)` → `MemorySummarize(node_id, summary)` | Summarize *toward* structure, not a blob. Rebounds temperature to 0.5. | *Summarize a cold node* |
| Re-sync after source files changed on disk | `MemoryCompile(path)` | Narrow the path — never the whole tree for one file. Honors `.remindb/ignore`. | *Recompile when the source drifts* |
| Connect two existing notes (no `[[Label]]` in source) | `MemoryRelate(source_id, target_label, target_source)` | Snapshot-free. Prefer `target_label`+`target_source` over `target_id` (IDs rotate on sibling reorder). | *MemoryRelate* |
| Remove a wrong / stale / never-belonged node | `MemoryForget(node_id, mode=strict\|cascade\|reparent)` | Mode picks what shape is left behind. Pinning does **not** protect from deletion. | *MemoryForget* |
| Undo several recent bad writes at once | `MemoryRollback(snapshot_id[, drop_after])` | Blast radius = every snapshot since target. `drop_after=true` is irreversible. One bad node → `MemoryForget` instead. | *MemoryRollback* |
| Protect an invariant from decay/summarization | `MemoryPin(node_id[, temperature])` | Snapshot-free; gates *cooling* only. Pin sparingly or the cold-set signal dies. | *MemoryPin / MemoryUnpin* |
| Author a durable cross-reference in content | `[[Label; w=2.5]]` in the payload | Bypassed inside code blocks / `<code>`. Weight = importance, not distance. | *Authoring wiki-links* |

## Why payload shape matters

A `MemoryWrite` payload is parsed as Markdown before storage. The parser is mechanical: heading levels build the tree spine; each non-heading block becomes one leaf node attached to the nearest open heading. **The shape of your Markdown determines the granularity of every future search, fetch, and delta** — a flat 500-word paragraph collapses to one fat unfetchable text node; the same content under H1/H2/H3 with lists becomes a dozen independently rankable, fetchable nodes. The parser won't fix a flat blob; you author indexing quality at write-time.

## How the parser maps Markdown to nodes

Grounded in `pkg/parser/markdown.go`. Block-level only — inline emphasis, links, and code spans flatten into the parent's content and aren't addressable.

| Markdown block | Becomes | Notes |
|---|---|---|
| `#`/`##`/`###` Heading | `heading` node — owns a subtree | Each level pops to the right ancestor (H2 after H3 closes the H3 subtree). DB depth ≈ heading level. |
| Bullet / ordered list | `list` node — single leaf | Items flattened to `- text` (nesting lost). One node, but each item ranks independently in FTS5. |
| Fenced code block | `code` node — single leaf | Language tag prepended as line 1. Empty blocks dropped. |
| Table | `table` node — single leaf | Tab-separated rows, header first. |
| Paragraph | `text` node — single leaf | Soft breaks → space, hard → newline. |
| HTML block | `text` node | Trimmed; empty dropped. |
| Frontmatter (`---\n…\n---` at start, YAML/TOML) | `preamble` node — one, before body | Good for tags / per-doc metadata. |
| Horizontal rule (`---` mid-doc) | **dropped silently** | Don't use `---` to separate sections — use a heading. |

Two consequences: **headings are the *only* tree-building block** (lists/tables are always leaves); **a payload with no headings has no spine** — every block attaches to the sentinel root at depth 1.

## Shape rules

1. **First line is the label.** Auto-derived, ≤80 chars, shown in `MemoryTree`/search. A blank or generic first line ("Notes:", "TODO") gives a useless label.
2. **Heading hierarchy splits a long note into addressable subtrees.** H1 = topic, H2 = aspect, H3 = fact. Below H4 rarely earns its keep.
3. **Lists for fact-sets**, not paragraphs. `- key: value` per line keeps each fact independently rankable even though the list is one node.
4. **Code blocks for snippets** you want verbatim — clean leaves, language tag preserved.
5. **Tables for matrices** — one leaf, but cells are searchable.
6. **No horizontal rules to separate sections** — the parser drops them. Use a heading.
7. **Don't merge unrelated facts into one paragraph** — split into list items or promote each to its own H3 under a shared H2.

## Examples — bad vs. good

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

`heading(Postgres…)` → 3× `heading(Region|Credentials|Schema)` → `list` + `text` + `text`. Each subtree independently fetchable; each fact ranks on its own. (A bare `# Prod DB facts` over a `- key: value` list is the compact variant of the same idea — see Shape rule 3. Never use `---` separators for grouping — Shape rule 6.)

## Shape also controls storage size — not just indexing

The same shape choices decide how many tokens a node **costs forever** — the parser compacts per node automatically:

- **Uniform records → TOON.** An array of same-keyed objects (config block, `key: value` rows, comparison matrix) is re-encoded in [TOON](../../docs/toon-encoding.md): shape stated once, values stream — ~40% smaller than YAML/JSON. Kept only on a ≥15% win.
- **MathML → LaTeX.** `<math>…</math>` in HTML is rebuilt as LaTeX by the same ≥15% rule; lossy conversions keep the raw MathML. See [MathML → LaTeX](../../docs/mathml-latex.md).

There is no encoding knob. You only control whether the win is *available*: a real table / uniform `- key: value` list **can** be TOON-compacted; the same data as a prose paragraph **cannot**. Math as `<math>` (or LaTeX) stays compact; hand-expanded into a sentence it stays a sentence. The `format` column records which won — you never author or query it.

**Consequence:** a node's `token_count` can be far below its raw byte size. That's compaction, not truncation — content is whole. Don't `MemoryFetch` a compact table/equation and "re-expand" it into prose on the next write; you'd undo the saving and fragment the index.

## MemoryWrite

**Create:**

```
remindb__MemoryWrite(payload="<full, structured content>")
```

- `anchor` omitted/empty → new node; content-addressed ID from the payload (xxhash64).
- Default `node_type` = `text` unless the payload starts with a heading. Default `source` = `mcp:write`. Default `depth` = 1 (top-level child of root).

### Update an existing node

```
remindb__MemoryWrite(anchor="<node_id>", payload="<full replacement>")
```

- Replaces content **in place**; `node_type`, `parent_id`, `source` preserved.
- Whole-payload replacement — no append, no patch. To extend: `MemoryFetch`, edit, write back.

### Search-first rule

Before creating, `MemorySearch` (via `remind`) for an existing anchor on the topic. Updating beats creating: existing parent/type/source/children stay attached, temperature history is preserved (a fresh node starts at default warmth), the diff trail records how the fact evolved. A near-duplicate sibling fragments the tree and returns near-duplicate hits.

### One logical note per call

Every write snapshots. Don't write per-keystroke. Three independent facts = three calls, each one coherent note. Related facts → one well-structured payload (headings + lists) → a clean subtree.

## MemoryForget — explicit node removal

For a node that's wrong/stale/never belonged — gone without rebuilding via `MemoryCompile` and without polluting history via an empty overwrite. One snapshot per call → recoverable through `MemoryHistory`, visible to `MemoryDelta`. Three mutually exclusive modes; pick by the shape you want left behind.

```
remindb__MemoryForget(node_id="<id>")                      # strict (default)
remindb__MemoryForget(node_id="<id>", mode="cascade")
remindb__MemoryForget(node_id="<id>", mode="reparent")
```

- **strict** (default) — deletes iff no children. With children: fails `node <id> has N children; pass mode=cascade or mode=reparent`, nothing changes. The right default — forces explicit thought about descendants.
- **cascade** — deletes target + every descendant. One `rem` per removed node in subtree order (server walks the subtree so each is visible to `MemoryDelta`), then deletes via the `parent_id ON DELETE CASCADE` FK; FTS5/relations triggers sync per row. Use when a whole branch is obsolete.
- **reparent** — deletes target, re-parents its direct children to the target's parent. Snapshot: one `rem` (target) + one `mod` per moved child with `OldHash == NewHash` (structural-only — `remind` covers the read side). Children update *before* the target deletes (else the cascade FK eats them — explains why mods list before the rem). **Root case:** `parent_id IS NULL` → children become roots (part of the contract).

Pinning does **not** block deletion — `MemoryForget` ignores `pinned`; the safeguard is not calling it. Relations self-heal: outgoing edges drop via FK cascade; incoming edges demote to pending (keyed by deleted label+file) and re-resolve on the next compile if a same-label node reappears.

## MemoryRollback — revert to a snapshot

When multiple recent writes left the graph bad and restoring a known-good point beats hand-patching. Takes a target `snapshot_id` (find via `MemoryHistory`/`MemoryStats`), emits **one new snapshot** of the rolled-back state (visible to `MemoryDelta`). Idempotent fast paths skip the emit: already at HEAD → `"already at snapshot <id>; nothing to do"`; computed state already matches HEAD → `"no rollback applied; computed state matches HEAD for snapshot <id>"`.

```
remindb__MemoryRollback(snapshot_id=42)                       # drop_after=false (default)
remindb__MemoryRollback(snapshot_id=42, drop_after=true)
```

- **`drop_after=false`** — keeps discarded snapshots as branched history (`target → … → prev_HEAD → rollback_snap`), still auditable via `MemoryHistory`. Use when the audit trail matters.
- **`drop_after=true`** — hard-deletes every snapshot + `diffs` row between target and rollback (`target → rollback_snap`). Use for noise/leaked secrets. **Irreversible** — another `MemoryRollback` won't find them.

**Restored** to target values: node content + hash; metadata (`parent_id`, `source_file`, `node_type`, `depth`, `label`, `format`, `token_count` — reparents/renames revert); tree shape (deleted nodes reappear, since-created removed); FTS5 (via triggers). **Not restored:** temperature / access count / last-accessed (access history, not content); pinned state (recreated nodes start unpinned); relations + pending relations (sideband — a `MemoryRelate` edge made between target and HEAD stays; one deleted with its endpoint re-resolves via pending on the next compile if the endpoint returns).

**Pre-migration limit:** a target older than `0005_diff_metadata` carries NULL old-metadata for `OpRem` events in range — those nodes can't be reconstructed; rollback **skips** them with a per-node warning (`pre-migration OpRem; node metadata unavailable`). Treat as actionable — recreate via `MemoryWrite` if important. Newer diffs always capture full metadata.

**Atomicity:** the whole flow (mutations, snapshot, diffs, cursor advance, optional prune) runs in one `Store.Tx` — a crash/cancel rolls back cleanly. This is why `MemoryRollback` bypasses `emitter.Emit` (two transactions for emit + prune would leave a real failure window). Choose rollback over `MemoryForget` for multiple bad writes / a polluted compile / privacy-sensitive content (`drop_after=true`); use `MemoryForget` for a single bad node (smaller blast radius, history intact).

## Authoring wiki-links — graph relations in your payload

A `[[Label]]` in a Markdown/HTML payload becomes a **parsed edge** (see `remind`'s "Relations" section). The parser strips resolver params from the stored content and captures them as edge metadata; readers see the clean normalized `[[Label]]`.

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

## MemoryPin / MemoryUnpin — protect a node from decay

Some content shouldn't cool: a project invariant, a stable fact, a flagged root. Decay would eventually push it below `ColdThreshold` into the cold-node stream and prompt a summarization that compacts wording you wanted verbatim. Pinning gates that.

```
remindb__MemoryPin(node_id="<id>")
remindb__MemoryPin(node_id="<id>", temperature=0.9)   # pin AND set 0.9 in one call
remindb__MemoryUnpin(node_id="<id>")
```

- Skipped by the decay sweep — `temperature` freezes at its current value, or at the optional override.
- Excluded from the cold-set query — never appears in a `remindb.temperature` notification.
- Boosts still apply (reads warm it normally — pin gates *cooling*, not access).
- `MemoryWrite` updates still work and snapshot; `pinned` is independent of content.

Optional `temperature` ∈ `[0,1]` for when the current value doesn't match the importance to lock in (pin at `0.9` for an invariant, `0.5` for neutral). `MemoryUnpin` has no temperature option — releasing returns a node to the lifecycle, not a temperature reset.

**Snapshot-free** — pin state is metadata, not content; not in `MemoryDelta`/`MemoryHistory`/cursor (same carve-out as `MemoryRelate`). **Pin:** stable architectural headings, invariant preamble/frontmatter, a user-flagged canonical summary. **Don't pin:** working notes that should decay, an entire compiled file (kills the cold-set signal), or "just in case" — default is unpinned; pinning is an explicit "this matters."

## Summarize a cold node — the notification handoff

`remind` describes the notification (`level: "warning"`, `logger: "remindb.temperature"`, `message: "Cold nodes detected; consider summarizing via MemorySummarize"`, `nodes: [{id,label,file,temperature}, …]`). Walk the `nodes` array and compact each:

```
remindb__MemoryFetch(anchor="<id>", budget=1500)                       # read what's there
remindb__MemorySummarize(node_id="<id>", summary="…")                  # replace; rebounds to 0.5
remindb__MemorySummarize(node_id="<id>", summary="…", temperature=0.7) # override when high-value
```

`MemorySummarize`: replaces content, recomputes `token_count`, rewrites label to `"Summary: <first line>"` (≤70 chars incl. prefix); **preserves `node_type`, `parent_id`, source**; **bumps temperature to `SummarizeRebound` (default 0.5)** so it leaves the cold set immediately (optional `temperature` ∈ `[0,1]` overrides); snapshots (prior wording recoverable via `MemoryHistory`).

Same shape rules apply to the `summary` — give it headings/a list; a dense paragraph is what you're compacting *away from*. The summary should index *better* than the original, not just be shorter. Notifications dedup per `ColdNotifyTTL` (default 1 hour); the next reminder only arrives if the node decays back below `ColdThreshold`. When a deployment sets `temperature.enabled: false` the ticker is frozen — no cold notifications fire, so this handoff simply never triggers until temperature is re-enabled (you can still summarize a node proactively).

## Recompile when the source drifts

`MemoryCompile` snapshots — same write semantics as `MemoryWrite`.

```
remindb__MemoryCompile(path="<file or subdir>", message="<optional snapshot note>")
```

Use when disk changed outside the rescan loop (external edit, disabled watcher, fresh `git pull`). **Prefer narrow paths** — one file is milliseconds; the whole tree is slow and creates a large snapshot. `path` may be absolute or relative; the server re-anchors it to `REMINDB_SOURCE` so the form you pass doesn't fork duplicate nodes (paths outside the root, or with `REMINDB_SOURCE` unset, pass through).

A `.remindb/ignore` at the source root is honored by compile + rescan — gitignore-style subset (literals, `*`/`?`/`[abc]`, trailing `/` dir-only, leading `/` root-anchor, `**` any-segment, `!` negation last-match-wins, `\` escape, `#` comments). Patterns subtract from the supported-extension allow-list; they can't re-include hardcoded skip dirs (`node_modules`, `vendor`, `target`, `dist`, `venv`) or dotfiles. Operators set this once — the agent doesn't author it.

## Common traps

Beyond the playbook watch-outs and section rules, these are the easiest to get wrong:

- **Empty-content overwrite is not deletion.** It sits at default warmth, pollutes search, leaves a `MemoryTree` phantom, and loses the "deleted" diff signal. Use `MemoryForget`.
- **`mode=cascade` ≠ `mode=reparent`.** Cascade discards the whole subtree; reparent keeps the children under the target's parent. If only the target is wrong (bad heading/level), reparent — cascade is the nuclear option.
- **You can't reparent by rewriting the heading hierarchy.** `MemoryWrite` with an anchor preserves `parent_id`; payload headings only affect content, not tree position. Use `MemoryForget mode=reparent`.
- **Label-only links collide.** First-match is deterministic but "first" = earliest by `(source_file, depth, id)`. If a label is shared across files, qualify with `source=`.
- **Never put a secret in a payload.** Record its *location* (the vault path / 1Password entry), not its value — the location is the searchable, safe form anyway.
