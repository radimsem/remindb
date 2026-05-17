---
name: remind
description: Read memory from a remindb MCP server — orient, search, fetch (single or batched), resync, traverse the relations graph, inspect DB health, read passive resources (`remindb://overview`). Covers the node/snapshot/temperature/relations model and FTS5 query syntax. Pair with `memoize` for writes.
---

# Remind — read from remindb so you don't re-grep

remindb is a compiled SQLite view of a workspace, served over MCP as the `Memory*` tool suite. It's long-term memory for your session — call it instead of re-reading files or grepping.

Read path (this skill): `MemoryTree`, `MemorySearch`, `MemoryFetch`, `MemoryFetchBatch`, `MemoryDelta`, `MemoryDiff`, `MemoryHistory`, `MemoryRelated`, `MemoryStats`, plus read-only **resources** (`remindb://overview`). Write path (pair with **`memoize`**): `MemoryWrite`, `MemoryForget`, `MemorySummarize`, `MemoryCompile`, `MemoryRelate`, `MemoryPin`, `MemoryUnpin`, `MemoryRollback`.

## Use-case playbook

Start here. Match your situation to a row, run the sequence, heed the watch-out; drop into the linked section only for the mechanics.

| When you need to… | Call | Watch out for | Section |
|---|---|---|---|
| Orient in a new or forgotten workspace | `MemoryTree(depth=5)` | One call per orientation — not before every search. Don't `ls`/`Glob`/`Grep`. | *The four read patterns* §1 |
| Find a fact or answer a question | `MemorySearch(query, budget)` | Send a keyword list, not a sentence. Always pass a budget. | *Search-query syntax*; *patterns* §2 |
| Read the full content of one hit | `MemoryFetch(anchor, budget)` | `depth=32` default is usually right; raise only for huge subtrees. | *patterns* §2 |
| Read the content of many hits at once | `MemoryFetchBatch(node_ids, budget)` | Use for any search/tree/delta result set. 256-id cap; no ancestors/children. | *patterns* §2 |
| Resume after time away or external writes | `MemoryDelta(since_snapshot=<last id>)` | It's the snapshot **id** (int64), not `cursor_hash`. Upper bound is always HEAD. | *patterns* §3 |
| Compare two fixed points (rollback target vs result, yesterday vs today) | `MemoryDiff(from_snapshot_id, to_snapshot_id)` | Both ends fixed — *not* `MemoryDelta`. `from` exclusive, `to` inclusive. | *patterns* §3 |
| Follow a `[[Label]]` seen in fetched content | `MemoryRelated(anchor, direction)` | Relations never appear in `MemoryDelta`/`MemoryHistory` — this is the only way to see the graph. | *patterns* §4 |
| Trace how one node evolved (to cite, or before a rewrite) | `MemoryHistory(anchor)` | Read-only; the rewrite itself is a `memoize` action. | *Inspect history* |
| Sanity-check the database (fresh session, odd results) | `MemoryStats()` | Free and read-only — use it whenever in doubt. | *Health check* |
| A `remindb.temperature` warning notification arrived | Hand to `memoize`: `MemoryFetch` → `MemorySummarize` | Won't re-fire for the same node until it warms and re-cools. | *Handing off to `memoize`* |

## Mental model

### Nodes

The smallest unit of memory is a **node**:

- **ID** — 11-char base62 (e.g. `3kGXxidmWBp`), content-addressed via xxhash64. The anchor for all follow-up calls; never guess or edit it.
- `parent_id` — nodes form a tree.
- `label` — scannable title (first meaningful line, ≤80 chars).
- `node_type` — `heading`, `list`, `kv`, `table`, `preamble`, `text`, `code`, `embed`. Hints shape, not behavior. `embed` = external HTML resource (image/video/audio/iframe). Inline `<svg>`/`<canvas>` → `code` with `format` = tag name. MathML → `code` with `format` = `latex` (converted) or `mathml` (raw kept). The `format` column records the medium.
- `token_count` — estimated cl100k-base tokens; the query layer honors budgets by it. Already reflects automatic per-node compaction (TOON for uniform data, LaTeX for MathML — see `memoize`), so a node can cost far fewer tokens than its raw bytes. That's compaction, not truncation — content is whole.
- `temperature` ∈ `[0.0, 1.0]` — warmth. Reads boost `+0.15` (capped at 1.0). A tick (default 5 min) decays everything by `factor = exp(-0.05 × elapsed_hours)` (~5%/hr). Two thresholds, both default `0.1`, **independent knobs**: `ColdThreshold` drives the cold-set *query* + search ranking floor; `NotifyThreshold` drives the cold-node *push*. A deployment can tune them separately.

### Snapshots

Every `MemoryCompile`/`MemoryWrite` creates a **snapshot**: an auto-increment `id` (int64) + a `cursor_hash` (xxhash64 of whole DB state). Linear parent chain. Pass the **id** to `MemoryDelta`; the **hash** is an opaque fingerprint for equality comparison only — they are not interchangeable.

### Diffs

Each snapshot carries per-node diffs (`add`/`mod`/`rem`, old+new content preserved). `MemoryDelta` = diffs since a known snapshot (upper bound always HEAD); `MemoryDiff` = state-vs-state between two arbitrary snapshots (git-diff hunks); `MemoryHistory` = the diff trail for one node.

**Structural-only mods:** a `mod` with `old_hash == new_hash` means content unchanged but tree position moved. Main producer: `MemoryForget mode=reparent` (children rewired to the deleted node's parent — each shows as a content-identical mod alongside the target's `rem`). Seeing `old_content == new_content`? Look for a same-snapshot `rem`.

### Relations — the graph layer

Directed weighted edges beyond the parent/child tree. Two kinds:

- **Resolved** — `source → target` between real node IDs. From the parser (`[[Label]]` wiki-link in source) or `MemoryRelate` (a `memoize` tool).
- **Pending** — target unresolved at compile time (forward ref, typo, not yet compiled). Stored with the hint, retried every compile.

Fields: `weight` (`REAL`, default `1.0`; higher = more important; ranks `MemoryRelated`, filters via `weight_min`; not yet in `MemorySearch` ranking). `origin` = `parsed` or `manual` (both can coexist per pair; `UNIQUE(source, target, origin)`). Direction is one-way (Obsidian-style); backlinks via `direction=in`.

**Relations are a sideband** — they never appear in `MemoryDelta`/`MemoryHistory`, and `MemoryRelate` doesn't snapshot. Inspect the graph only via `MemoryRelated`. When a target is deleted, incoming edges go resolved→pending (label preserved); a same-label heading reappearing self-heals them on the next compile.

### Ranking

`score = relevance × (0.3 + 0.7 × temperature)`. Relevance = FTS5 BM25-like rank. A cold node with a great match still surfaces; a warm node with a weak match also surfaces. Budget trims from the bottom after ranking.

### Notifications

After each tick the server pushes a cold-node nudge to every client session that called `SetLoggingLevel`, over the MCP `notifications/message` channel (*not* stderr), `level: "warning"`, `logger: "remindb.temperature"`:

```json
{
  "message": "Cold nodes detected; consider summarizing via MemorySummarize",
  "suggested_action": "MemorySummarize",
  "nodes": [ { "id": "<11-char base62>", "label": "...", "file": "...", "temperature": 0.07 } ]
}
```

Dedup with hysteresis: a node is notified once when it drops below `NotifyThreshold`, suppressed until it warms above and re-cools. Treat it as a direct cue to `MemorySummarize` the listed `id`s — `memoize` owns that workflow (`MemoryFetch` then `MemorySummarize`).

The whole stream can be **frozen at runtime**: setting `temperature.enabled: false` in `.remindb/config.json` makes the ticker perform no decay and push no notifications (live-reloaded at the next tick, no restart). Silence here may mean the brain is frozen, not that nothing is cold — it resumes on the next tick after `enabled` flips back to `true`.

### Budgets

Every read tool takes a `budget` (int, tokens); the engine fills to it and stops. Guidance: `500` scoped fact · `1000` topic exploration · `1500` broad sweep.

Operators can set per-tool defaults in `.remindb/config.json` under a `budgets` block (`search`, `fetch`, `fetch_batch`, `related`). Resolution per tool: explicit positive `budget` wins → configured default → built-in. `MemoryRelated` built-in is 1000; `MemorySearch`/`MemoryFetch`/`MemoryFetchBatch` treat unset as **unlimited**. Pass an explicit `budget` when response size matters; don't assume a server default is configured.

## Search-query syntax — critical

Search goes through SQLite FTS5. Pre-processing: **bare multi-word queries are rewritten to `OR` between each word**; anything that already looks like FTS5 passes through unchanged.

The server checks for any of: `OR  AND  NOT  NEAR(  "  :  *  (`

- Any present → pass through unchanged (already FTS5).
- Else → whitespace-split, joined with ` OR `.
- A single bare word → passed through.

```
"token bucket rate limit"  → token OR bucket OR rate OR limit   (matches ≥1 word, ranked by hit count)
"database"                 → database                            (passed through)
"token AND bucket"         → passed through                      (both required)
"\"token bucket\""         → passed through                      (exact adjacent phrase)
```

How to construct queries:

1. **Keyword lists, not sentences.** Strip function words ("how", "the", "do", "I") — they dilute OR ranking.
2. **Bare multi-word for broad recall** — "any-of" matching, ranked by how many words hit.
3. **FTS5 operators for precision:** `"exact phrase"` (adjacent, in order) · `a AND b` (both) · `a NOT b` (exclude b) · `prefix*` (prefix match) · `NEAR(a b, 5)` (within 5 tokens).
4. **Quote internal punctuation.** Hyphens/dots are tokenizer boundaries — search `"rate-limit"` quoted to match the hyphenated form.

```
# Bad  — stopwords dilute:  "how do I configure the rate limiter middleware"
# Good — keywords only:     "rate limiter middleware configure"
# Best — known phrase:      "\"rate limiter middleware\""
# All terms required:       "rate AND limiter AND redis"
```

## The four read patterns

### 1. Orient: tree first, always

Session start or task switch: call `MemoryTree` once. Don't `ls`/`Glob`/`Read`.

```
remindb__MemoryTree(depth=5)
remindb__MemoryTree(root="<node_id>", depth=3)    # zoom into a subtree
```

Returns a typed, labeled hierarchy with temperatures + token counts. Follow hot branches first. Default depth 5; raise only when shallow didn't reveal the anchor.

### 2. Look up: MemorySearch, then MemoryFetch or MemoryFetchBatch

Never grep. `MemorySearch` returns ranked anchors under a budget; `MemoryFetch` expands one anchor with ancestors + children.

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

### 3. Resync and compare: MemoryDelta / MemoryDiff

Picked by which end of the range is fixed.

**`MemoryDelta`** — "what changed since X?", upper bound always HEAD. Use on resume / after external writes; pass the last snapshot **id** seen:

```
remindb__MemoryDelta(since_snapshot=42)    # snapshot ID (int64), not cursor_hash
remindb__MemoryDelta(since_snapshot=0)     # all changes ever — expensive, rarely wanted
```

Returns `[op] node_id (snapshot N)` lines; fetch nodes you need. Keep the last snapshot id from a prior tree/search/write result.

**`MemoryDiff`** — "what changed between X and Y?", both ends fixed. Like `git diff X Y`: compares state-at-X vs state-at-Y, not the event log between. Lower bound exclusive, upper inclusive:

```
remindb__MemoryDiff(from_snapshot_id=40, to_snapshot_id=42)   # state(40) → state(42)
```

One git-diff block per **changed node**; intermediate jitter (`mod→mod→mod`, `add→mod`) collapses to the net change. Block = `[op] node_id` + unified-diff hunks (`@@`, `-`/`+`/context). Nodes ending where they started are dropped silently (like `git diff X Y`). `from > to` → validation error; `from == to` → `no changes`.

### 4. Traverse: MemoryRelated

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

## Health check: MemoryStats

Sanity-check the DB (fresh session, suspicious results, before a `MemoryCompile`):

```
remindb__MemoryStats()
```

Plain-text block: DB path + size, node count + per-`node_type` breakdown, total tokens, snapshot count + latest id/age/cursor, temperature spread (avg/median/hot/cold/pinned), relation count + per-`origin` breakdown (`parsed`/`manual`/`pending` when non-zero), FTS5 row count. Per-category counts hang off the total root, and the `Relations:` header is always the sum of every sub-branch (all origins + `pending:`) — it reconciles to the breakdown below it:

```
Nodes:             42 (1280 tokens)
    ├─ heading:    17
    └─ text:       13
Relations:         3
    ├─ manual:     2
    └─ pending:    1
```

Read-only — no `OpMu`, no boost, no payload in the call log. Use freely; one cheap roundtrip.

## Resources: passive read-only views

Beyond the `Memory*` tools, the server exposes MCP **resources** — URIs you read instead of call. `resources/list` enumerates them; `resources/read` returns the body. The one resource today:

```
remindb://overview   →   application/json
```

Same data as `MemoryStats`, but as the locked JSON envelope (`db_path`, `db_bytes`, `nodes{total,by_type,tokens}`, `snapshots{count,head_id,cursor_hash,latest_message,latest_age_s}`, `temperature{avg,median,hot,cold,pinned}`, `relations{total,by_origin,pending}`, `fts_rows`) — for programmatic consumers (a UI rendering the database), not for reasoning in prose.

The key difference from a read *tool*: a resource read is **passive observation**. It does **not** boost temperature. Reach for `MemorySearch`/`MemoryFetch` when you want the node to count as accessed (and warm up); read the resource only when you explicitly must *not* perturb the heatmap. For ordinary "what's the DB state" curiosity in a session, `MemoryStats` is still the call — the resource exists for external renderers.

## Inspect history before rewriting

Before a `memoize`-side overwrite, check how a node evolved:

```
remindb__MemoryHistory(anchor="<node_id>", depth=10)
```

Snapshot-ordered `add`/`mod`/`rem` with truncated old+new content. Use it to roll back (re-write the `old` payload via `memoize`'s `MemoryWrite`) or cite prior wording.

## Handing off to `memoize`

This skill stops where mutation begins. Four triggers send you to `memoize`:

- **User asks to remember/save/note something** → `MemoryWrite` + the Markdown-shape rules.
- **A `level: "warning"` / `logger: "remindb.temperature"` notification** → `MemoryFetch` → `MemorySummarize` compaction.
- **Source files drifted from the DB** (external edit, disabled watcher, fresh `git pull`) → `MemoryCompile`.
- **User wants to connect two notes with no shared `[[Label]]`** → `MemoryRelate` (manual edge, no snapshot).

## Common traps

Each is stated in its section above; collected here because they're the easiest to get wrong:

- **snapshot id ≠ cursor_hash.** `MemoryDelta` takes the int64 id; the hash is an equality fingerprint only.
- **`MemoryDelta` ≠ `MemoryDiff`.** Delta's upper bound is always HEAD; Diff fixes both ends. Want a forensic between-snapshots comparison → Diff.
- **`ColdThreshold` ≠ `NotifyThreshold`.** Both default `0.1` but are independent — one drives the cold-set query, the other the push.
- **Relations are invisible to `MemoryDelta`/`MemoryHistory`.** The diff trail is node-content only; call `MemoryRelated` for the graph.
- **A `[[Label]]` in fetched content is a cue, not decoration.** Call `MemoryRelated` before responding.
- **Don't re-read what's already in remindb.** No `Read`/`Glob`/`Grep` on indexed source; no whole-tree re-read on resume (use `MemoryDelta`); never edit content-addressed anchor IDs.
