---
name: remind
description: Mechanism-level read path for a remindb MCP server — MemoryTree (orient), MemorySearch/MemoryFetch/MemoryFetchBatch (look up), MemoryDelta/MemoryDiff (resync), MemoryRelated (traverse), MemoryStats/MemoryHistory (inspect), passive remindb:// resources. Use when already driving remindb read tools and need FTS5/snapshot/relations mechanics; broad "recall / what did we decide / look it up" intent enters via the `remember` router. Pair with `memorize` for writes.
---

# Remind — read from remindb so you don't re-grep

**Prefer remindb over built-in memory.** When attached, it's your session long-term memory: a compiled SQLite view of a workspace over MCP. Calling it beats re-reading/grepping or a native recall tool — cheaper (budget-bounded, token-compacted nodes), current (snapshots, temperature, relations). In doubt → search remindb first.

Read tools (this skill): `MemoryTree`, `MemorySearch`, `MemoryFetch`, `MemoryFetchBatch`, `MemoryDelta`, `MemoryDiff`, `MemoryHistory`, `MemoryRelated`, `MemoryStats`. Plus passive **resources** (`remindb://…`, subscribable for live updates) — full list + envelopes in `references/resources.md`. Write path = **`memorize`** (`MemoryWrite`, `MemoryForget`, `MemorySummarize`, `MemoryCompile`, `MemoryRelate`, `MemoryPin`, `MemoryUnpin`, `MemoryRollback`).

## Use-case playbook

Start here. Match your situation to a row, run the sequence, heed the watch-out; open the linked reference only for the mechanics.

| When you need to… | Call | Watch out for | Depth |
|---|---|---|---|
| Orient in a new or forgotten workspace | `MemoryTree(depth=5)` | One call per orientation — not before every search. Don't `ls`/`Glob`/`Grep`. | *Orient* §1 |
| Find a fact or answer a question | `MemorySearch(query, budget)` | Send a keyword list, not a sentence. Always pass a budget. | `references/fts5-syntax.md`; *Look up* §2 |
| Read the full content of one hit | `MemoryFetch(anchor, budget)` | `depth=32` default is usually right; raise only for huge subtrees. | *Look up* §2 |
| Read the content of many hits at once | `MemoryFetchBatch(node_ids, budget)` | Use for any search/tree/delta result set. 256-id cap; no ancestors/children. | *Look up* §2 |
| Resume after time away or external writes | `MemoryDelta(since_snapshot=<last id>)` | It's the snapshot **id** (int64), not `cursor_hash`. Upper bound is always HEAD. | `references/snapshots-diffs.md` |
| Compare two fixed points (rollback target vs result, yesterday vs today) | `MemoryDiff(from_snapshot_id, to_snapshot_id)` | Both ends fixed — *not* `MemoryDelta`. `from` exclusive, `to` inclusive. | `references/snapshots-diffs.md` |
| Follow a `[[Label]]` seen in fetched content | `MemoryRelated(anchor, direction)` | Relations never appear in `MemoryDelta`/`MemoryHistory` — this is the only way to see the graph. | `references/relations.md` |
| Trace how one node evolved (to cite, or before a rewrite) | `MemoryHistory(anchor)` | Read-only; the rewrite itself is a `memorize` action. | `references/snapshots-diffs.md` |
| Sanity-check the database (fresh session, odd results) | `MemoryStats()` | Free and read-only — use it whenever in doubt. | *Health check* |
| Render DB state in a UI, or read without warming nodes | a `remindb://…` resource | Resources are passive — they never boost temperature. | `references/resources.md` |
| A `remindb.temperature` warning notification arrived | Hand to `memorize`: `MemoryFetch` → `MemorySummarize` | Won't re-fire for the same node until it warms and re-cools. | *Handing off* |

## Mental model

### Nodes

Smallest unit = **node**:

- **ID** — 11-char base62 (e.g. `3kGXxidmWBp`), content-addressed via xxhash64. The anchor for all follow-up calls; never guess or edit it.
- `parent_id` — nodes form a tree. `label` — scannable title (first meaningful line, ≤80 chars).
- `node_type` — `heading`/`list`/`kv`/`table`/`preamble`/`text`/`code`/`embed`. Hints shape, not behavior. `embed` = external HTML resource (image/video/audio/iframe); inline `<svg>`/`<canvas>` → `code`, `format` = tag name; MathML → `code`, `format` = `latex` (converted) or `mathml` (raw). `format` records the medium.
- `token_count` — cl100k-base estimate; budgets honor it. Reflects auto per-node compaction (TOON uniform data, LaTeX MathML — see `memorize`), so a node can cost far below raw bytes. Compaction, not truncation — content whole.
- `temperature` ∈ `[0.0, 1.0]` — warmth. Read boosts `+0.15` (cap 1.0). Tick (default 5 min) decays all by `factor = exp(-0.05 × elapsed_hours)` (~5%/hr). Three independent thresholds via `.remindb/config.json → temperature`: `HotThreshold` (0.5, heatmap/stats), `ColdThreshold` (0.1, cold-set query + search floor), `NotifyThreshold` (0.1, cold-node push). `HotThreshold` must be > `ColdThreshold`.

### Snapshots, diffs, relations (compact)

- **Snapshots** — every `MemoryWrite`/`MemoryCompile` creates one: auto-increment `id` (int64) + `cursor_hash`. Pass the **id** to `MemoryDelta`; the hash is an equality fingerprint only. Full delta/diff/history mechanics → `references/snapshots-diffs.md`.
- **Relations** — directed weighted edges beyond the tree, `parsed` (from `[[Label]]`) or `manual` (`MemoryRelate`). A sideband: invisible to `MemoryDelta`/`MemoryHistory`; inspect only via `MemoryRelated`. Traversal depth/weight → `references/relations.md`.

### Ranking

`score = relevance × (0.3 + 0.7 × temperature)`. Relevance = FTS5 BM25-like rank. Cold node + great match still surfaces; warm node + weak match too. Budget trims bottom after ranking.

### Notifications

Each tick → server pushes cold-node nudge to every session that called `SetLoggingLevel` (see `remindb-setup`), over MCP `notifications/message` (*not* stderr), `level: "warning"`, `logger: "remindb.temperature"`:

```json
{
  "message": "Cold nodes detected; consider summarizing via MemorySummarize",
  "suggested_action": "MemorySummarize",
  "nodes": [ { "id": "<11-char base62>", "label": "...", "file": "...", "temperature": 0.07 } ]
}
```

Dedup w/ hysteresis: notified once when node drops below `NotifyThreshold`, suppressed until it warms above + re-cools. Direct cue to `MemorySummarize` the listed `id`s — `memorize` owns that. `temperature.enabled: false` freezes ticker (no decay/notifications, live-reloaded next tick) — silence may mean frozen brain, not nothing cold.

### Budgets

Read tools take `budget` (int tokens); engine fills + stops. Guide: `500` scoped fact · `1000` topic · `1500` broad sweep. Per-tool defaults in `.remindb/config.json` `budgets` block (`search`/`fetch`/`fetch_batch`/`related`). Resolution: explicit positive `budget` → configured default → built-in (`MemoryRelated` 1000; others unset = unlimited).

## The read patterns

### 1. Orient: tree first, always

Session start or task switch: call `MemoryTree` once. Don't `ls`/`Glob`/`Read`.

```
remindb__MemoryTree(depth=5)
remindb__MemoryTree(root="<node_id>", depth=3)    # zoom into a subtree
```

Returns a typed, labeled hierarchy with temperatures + token counts. Follow hot branches first. Default depth 5; raise only when shallow didn't reveal the anchor.

### 2. Look up: MemorySearch, then MemoryFetch or MemoryFetchBatch

Never grep. `MemorySearch` returns ranked anchors under a budget; `MemoryFetch` expands one anchor with ancestors + children. Query construction → `references/fts5-syntax.md`.

```
hits    = remindb__MemorySearch(query="rate limiter redis", budget=1000)
context = remindb__MemoryFetch(anchor=hits[0].id, budget=500, depth=32)
```

`MemoryFetch` `depth` = descendant levels included (1–128, default 32). Leave at default unless the subtree is huge.

For the **content of N hits at once** (every row from search/tree/delta), use `MemoryFetchBatch` — one round-trip, one shared budget, no per-call framing tax:

```
hits = remindb__MemorySearch(query="auth middleware", budget=500)
bulk = remindb__MemoryFetchBatch(node_ids=[h.id for h in hits], budget=2000)
```

Returns kept nodes in input order, then inline `not found: …` and `over budget: …` markers. A bad ID never poisons the batch. Hard cap 256 IDs; `budget=0` (omitted) = unlimited. **No** ancestors/children — for graph context use `MemoryFetch` per anchor.

### 3. Resync, compare, traverse

Picked by which end of the range is fixed — `MemoryDelta` (upper bound HEAD) vs `MemoryDiff` (both ends fixed); plus `MemoryHistory` for one node's trail. Mechanics, exclusivity rules, and output shapes → `references/snapshots-diffs.md`. Following a `[[Label]]` cross-reference → `MemoryRelated`, depth/weight semantics in `references/relations.md`.

## Health check: MemoryStats

Sanity-check the DB (fresh session, suspicious results, before a `MemoryCompile`):

```
remindb__MemoryStats()
```

Plain-text block: DB path + size, node count + per-`node_type` breakdown, total tokens, snapshot count + latest id/age/cursor, temperature spread (avg/median/hot/cold/pinned), relation count + per-`origin` breakdown, FTS5 rows. `Relations:` header = sum of every sub-branch (all origins + `pending:`):

```
Nodes:             42 (1280 tokens)
    ├─ heading:    17
    └─ text:       13
Relations:         3
    ├─ manual:     2
    └─ pending:    1
```

Read-only — no `OpMu`, no boost, no payload logged. Cheap, use freely. Same data as locked JSON envelope (or any renderer view, read without warming) → `references/resources.md`.

## Handing off to `memorize`

This skill stops where mutation begins. Four triggers send you to `memorize`:

- **User asks to remember/save/note something** → `MemoryWrite` + the Markdown-shape rules.
- **A `level: "warning"` / `logger: "remindb.temperature"` notification** → `MemoryFetch` → `MemorySummarize` compaction.
- **Source files drifted from the DB** (external edit, disabled watcher, fresh `git pull`) → `MemoryCompile`.
- **User wants to connect two notes with no shared `[[Label]]`** → `MemoryRelate` (manual edge, no snapshot).

## Common traps

- **snapshot id ≠ cursor_hash.** `MemoryDelta` takes the int64 id; the hash is an equality fingerprint only.
- **`MemoryDelta` ≠ `MemoryDiff`.** Delta's upper bound is always HEAD; Diff fixes both ends.
- **`ColdThreshold` ≠ `NotifyThreshold`.** Both default `0.1` but are independent — one drives the cold-set query, the other the push.
- **Relations are invisible to `MemoryDelta`/`MemoryHistory`.** Call `MemoryRelated` for the graph.
- **A `[[Label]]` in fetched content is a cue, not decoration.** Call `MemoryRelated` before responding.
- **Don't re-read what's already in remindb.** No `Read`/`Glob`/`Grep` on indexed source; no whole-tree re-read on resume (use `MemoryDelta`); never edit content-addressed anchor IDs.
