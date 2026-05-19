---
name: remind
description: Read memory from a remindb MCP server — orient, search, fetch (single or batched), resync, traverse the relations graph, inspect DB health, read passive resources (`remindb://overview`, `remindb://files`, `remindb://tree`, `remindb://graph`, `remindb://snapshots`, `remindb://temperature`, `remindb://doctor`, `remindb://logs`, `remindb://sessions`, `remindb://sessions/history`, `remindb://sessions/logs`, `remindb://rescan`), subscribe for coalesced live resource updates. Covers the node/snapshot/temperature/relations model and FTS5 query syntax. Pair with `memoize` for writes.
---

# Remind — read from remindb so you don't re-grep

remindb is a compiled SQLite view of a workspace, served over MCP as the `Memory*` tool suite. It's long-term memory for your session — call it instead of re-reading files or grepping.

Read path (this skill): `MemoryTree`, `MemorySearch`, `MemoryFetch`, `MemoryFetchBatch`, `MemoryDelta`, `MemoryDiff`, `MemoryHistory`, `MemoryRelated`, `MemoryStats`, plus read-only **resources** (`remindb://overview`, `remindb://files`, `remindb://tree`, `remindb://graph`, `remindb://temperature`, `remindb://doctor`, `remindb://logs`, `remindb://sessions`, `remindb://sessions/logs`, `remindb://rescan`), which a renderer can also **subscribe** to for coalesced live updates. Write path (pair with **`memoize`**): `MemoryWrite`, `MemoryForget`, `MemorySummarize`, `MemoryCompile`, `MemoryRelate`, `MemoryPin`, `MemoryUnpin`, `MemoryRollback`.

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

Beyond the `Memory*` tools, the server exposes MCP **resources** — URIs you read instead of call. `resources/list` enumerates them; `resources/read` returns the body. The resources today:

```
remindb://overview          →   application/json
remindb://files             →   application/json
remindb://tree              →   application/json   (full node hierarchy)
remindb://tree/{rootId}?depth=N →   application/json   (bounded subtree; templated)
remindb://graph             →   application/json   (relations graph)
remindb://snapshots         →   application/json   (full version history)
remindb://snapshots?limit=N →   application/json   (newest N; templated)
remindb://snapshots/{id}/diffs →   application/json   (per-snapshot diffs; templated)
remindb://temperature       →   application/json   (per-node heatmap + summary)
remindb://doctor            →   application/json   (health-check report)
remindb://logs              →   application/json   (recent server log records)
remindb://sessions          →   application/json   (active MCP client sessions)
remindb://sessions/history  →   application/json   (durable per-client session ledger)
remindb://sessions/history/{hash}  →  application/json  (one client's ledger)
remindb://sessions/logs     →   application/json   (per-session logfile index)
remindb://sessions/logs/{id}  →  application/json   (one session's captured trace; templated)
remindb://rescan            →   application/json   (latest source-rescan tick)
```

`remindb://overview` — same data as `MemoryStats`, but as the locked JSON envelope (`db_path`, `db_bytes`, `nodes{total,by_type,tokens}`, `snapshots{count,head_id,cursor_hash,latest_message,latest_age_s}`, `temperature{avg,median,hot,cold,pinned}`, `relations{total,by_origin,pending}`, `fts_rows`) — for programmatic consumers (a UI rendering the database), not for reasoning in prose.

`remindb://files` — the JSON twin of `remindb inspect --files`: compiled source files grouped by compile root with per-file node and token counts. Reading it (URI `remindb://files`) returns:

```json
{
  "roots": [
    { "root": "/repo/docs", "files": [ { "path": "docs/architecture.md", "nodes": 12, "tokens": 840 } ] },
    { "root": "", "files": [ { "path": "scratch.md", "nodes": 1, "tokens": 30 } ] }
  ]
}
```

Roots sort ascending; the empty-string root (files with no compile root) sorts last. Powers a desktop file explorer — again, a renderer's view, not a reasoning call.

`remindb://tree` — the structured twin of `MemoryTree`: the full parent/child hierarchy as nested JSON instead of indented text, for a UI that draws the tree. The templated form `remindb://tree/{rootId}?depth=N` returns just the subtree under `rootId`, bounded to `N` descendant levels (omit `?depth` for the whole subtree). Each node carries `id, type, label, depth, tokens, temperature, source, children`:

```json
{
  "roots": [
    { "id": "aB3", "type": "heading", "label": "Architecture", "depth": 0,
      "tokens": 120, "temperature": 0.42, "source": "docs/architecture.md",
      "children": [
        { "id": "aB3-1", "type": "text", "label": "Overview", "depth": 1,
          "tokens": 80, "temperature": 0.31, "source": "docs/architecture.md", "children": [] }
      ] }
  ]
}
```

`roots` is always present (`[]` on an empty DB); an unknown `rootId` is an error, not an empty body. Use `MemoryTree` when you want the access to warm the nodes — this resource is for rendering only.

`remindb://graph` — the relations knowledge graph (the "brain" view) as locked JSON, for a UI that draws it. `nodes` is the referenced set only (anything that is an endpoint of a resolved edge or the source of a pending one — orphans are excluded; use `remindb://tree` for the full hierarchy), `edges` are resolved relations (`source→target`, with `weight` and `origin` = `parsed`|`manual`), `pending` are unresolved edges kept as a distinct array (the `source` exists but the target is only a `target_label`/`target_source`/`target_id_hint`, never a resolved id):

```json
{
  "nodes":   [ { "id": "aB3", "label": "Architecture", "type": "heading", "temperature": 0.42 } ],
  "edges":   [ { "source": "aB3", "target": "aB3-1", "weight": 4.2, "origin": "parsed" } ],
  "pending": [ { "source": "aB3", "target_label": "Roadmap", "target_source": "",
                 "target_id_hint": "", "weight": 1.0, "origin": "parsed" } ]
}
```

All three keys are always present (`{"nodes":[],"edges":[],"pending":[]}` on an empty DB). It mirrors `MemoryRelated`'s data without the traversal — `MemoryRelated` walks the graph from an anchor and warms what it touches; this resource is the whole static graph for rendering, and warms nothing.

`remindb://snapshots` — the version history behind `MemoryHistory`, every snapshot newest-first with the parent links that reconstruct branch topology, for an interactive timeline UI. `remindb://snapshots?limit=N` bounds it to the newest N (omit for full history); `remindb://snapshots/{id}/diffs` returns one snapshot's diff records (`op, node_id, old_hash, new_hash, old_content, new_content`), the data behind `MemoryDelta`:

```json
{ "snapshots": [
  { "id": 3, "parent_id": 2, "message": "write:aB3", "compile_root": "/repo", "created_at": 1737072000, "is_head": true },
  { "id": 1, "parent_id": null, "message": "compile", "compile_root": "/repo", "created_at": 1737070000, "is_head": false }
] }
```

`parent_id` is `null` for a root snapshot (never `0`); at most one snapshot is `is_head`. `snapshots`/`diffs` are always present (`[]` on an empty DB); a bad `{id}` or non-positive `?limit` is an error, not an empty body. It mirrors `MemoryHistory`/`MemoryDelta` for rendering — use those tools when you want the access to warm nodes.

`remindb://temperature` — the heatmap view: every node in one `nodes` array (hot, cold, pinned all together — the renderer classifies from `temperature` vs the echoed cut points), plus an aggregate `summary`. Hot/cold counts mirror `MemoryStats`, except `cold` uses the **live configured** `cold_threshold` (`.remindb/config.json` → `temperature.cold_threshold`), not a hardcoded one; `hot_threshold` is the fixed `0.5` presentation cut:

```json
{ "summary": { "avg": 0.29, "median": 0.30, "hot": 1, "cold": 2, "pinned": 1,
               "cold_threshold": 0.1, "hot_threshold": 0.5 },
  "nodes":   [ { "id": "aB3", "label": "Auth design", "temperature": 0.8, "pinned": false } ] }
```

`nodes` is always present (`[]` on an empty DB) and unified — there is no separate cold list; `summary` echoes the thresholds so a renderer reproduces the exact hot/cold classification. It does **not** boost — reading the heatmap must not warm the nodes it measures.

`remindb://doctor` — the health-check report, byte-equivalent to `remindb doctor --json`: an overall worst-wins `status` header (`pass`/`warn`/`fail`) plus every check's `name`/`status`/`detail`, for a desktop client rendering the health panel without shelling out to the CLI. Read-only — it runs the same checks `doctor` does but never applies `--fix`:

```json
{ "status": "warn",
  "checks": [
    { "name": "fts5_sync", "status": "pass", "detail": "FTS5 index in sync with 12 nodes" },
    { "name": "stale_compile_root", "status": "warn", "detail": "1/2 compile roots no longer exist: [/old/repo]" }
  ] }
```

`status` is the worst check status across the report (`fail` beats `warn` beats `pass`); `checks` is always present and ordered. It reuses `pkg/doctor` directly — no duplicated check logic — and, like every resource, warms nothing.

`remindb://logs` — the recent server log records from a bounded in-memory ring buffer, for a desktop log console. `records` is always present (`[]` before anything is logged), ordered oldest-first (**newest last**); `dropped` counts records evicted once the buffer filled past its capacity (`server.logging.buffer_size`, default 1000):

```json
{ "records": [ { "time": 1737200000123, "level": "INFO", "msg": "serve: starting", "attrs": { "db": "mem.db" } } ],
  "dropped": 0 }
```

`time` is Unix milliseconds; `level` is the slog level string (`DEBUG`/`INFO`/`WARN`/`ERROR`); `attrs` is the flattened structured fields (always an object). It mirrors exactly what stderr/file logging emits — `--verbose`/level filtering applies upstream, so below-level records never appear here. Payloads/bodies are never logged, so they never reach this resource.

`remindb://sessions` — the MCP client sessions attached to *this* `serve` process (the one bound to `db_path`), for a "who's attached to this brain" view. `sessions` is always present (`[]` when none attached), ordered oldest-connected first; membership mirrors the SDK's live session set (a closed session disappears on the next read):

```json
{ "db_path": "/repo/.remindb/memory.db",
  "sessions": [ { "id": "k7f3…",
                  "client_meta": { "name": "claude-code", "version": "1.2.0", "protocol": "2025-06-18" },
                  "transport": "stdio",
                  "connected_at": 1737200000, "last_activity": 1737200042, "count_tool_calls": 9 } ] }
```

`client_meta` (always present) carries `name`/`version`/`protocol` from the `initialize` handshake (`title` omitted when unset) — **self-reported, display-only, not identity**. `connected_at`/`last_activity` are Unix **seconds**; `count_tool_calls` counts `tools/call` only (resource reads and pings are not tool calls); `listen` (HTTP bind address) appears only for `http` sessions and is omitted for stdio.

`remindb://sessions/history` — the durable counterpart: every client that has *ever* attached, accumulated across reconnects and `serve` restarts (in-memory `sessions` resets on restart; this one persists to `.remindb/sessions/`). `clients` is always present (`[]` when empty); each entry has a stable `hash` (content hash of the client identity tuple — **not** the spoofable `client.name`), last-seen `client`, `sessions` count, summed `lifetime_seconds`, `last_disconnect` (Unix seconds, 0 if none closed), and lifetime `tool_calls`. `remindb://sessions/history/{hash}` returns one client's bare object (the `hash` from the array), erroring on an unknown hash. A crashed session loses ≤ one flush interval; a reconnect never double-counts. Pure on-disk projection — no boost, lock, or snapshot.

`remindb://sessions/logs` — the read surface over the per-session logfiles under `.remindb/logs/`, for auditing one MCP client's tool-call + `Warn`/`Error` trace. The static URI is the index (`{db_path, logs[]}` where each log is `{session_id, size_bytes, rotated, modified_at}`; `logs` is `[]` when session logging is off); `remindb://sessions/logs/{id}` returns `{session_id, entries}` where each entry is the structured `{time, level, msg, fields}` in append order (newest last), **active file only** (a rotated `.1` tail is flagged in the index but not replayed). Unknown id → clean error. JSONL on disk, deserialized through the same `Record` type `render()` writes — no parser drift. Pure file read: no boost, lock, or snapshot.

`remindb://rescan` — the latest tick of the `serve` source-rescan loop, for a live rescan-activity panel. Always present; before the first tick (or when `serve` runs with no `--source`) `last_meta` is the zero value with `run_at: 0`:

```json
{ "interval_s": 30,
  "last_meta": { "run_at": 1737200000, "error": "",
                 "added": 2, "modified": 1, "removed": 0,
                 "purged_files": [ { "path": "notes/old.md", "nodes": 4 } ] } }
```

`interval_s` is the configured rescan interval in seconds (reflects live `.remindb/config.json` reloads). `last_meta` is one tick's result: `run_at` is Unix **seconds** (0 = never run); `error` is the last tick's failure string (empty on success); `added`/`modified`/`removed` are that tick's compile counts (sum them yourself — there is no `total`); `purged_files` lists each source file deleted from disk that tick with how many context nodes it carried (`[]` when nothing was purged — purging is whole-file only, so the per-file node count fully describes it).

The key difference from a read *tool*: a resource read is **passive observation**. It does **not** boost temperature. Reach for `MemorySearch`/`MemoryFetch` when you want the node to count as accessed (and warm up); read the resource only when you explicitly must *not* perturb the heatmap. For ordinary "what's the DB state" curiosity in a session, `MemoryStats` is still the call — the resource exists for external renderers.

### Subscriptions: live updates instead of polling

A renderer that wants to stay current `resources/subscribe`s to a URI and gets `notifications/resources/updated` when its state changes — no polling loop. Subscribable URIs and what triggers them:

```
remindb://graph        ← write · summarize · compile · forget · rollback · relate
remindb://snapshots    ← write · summarize · compile · forget · rollback · rescan
remindb://tree         ← write · summarize · compile · forget · rollback · rescan
remindb://files        ← compile · rescan
remindb://rescan       ← a source-rescan tick that mutated the store
remindb://temperature  ← a temperature tick that decayed a node
remindb://logs         ← a new server log record
```

```
resources/subscribe   { "uri": "remindb://graph" }
→ later, on change:   notifications/resources/updated  { "uri": "remindb://graph" }
resources/unsubscribe { "uri": "remindb://graph" }
```

`overview`, `doctor`, `sessions`, and the templated forms are **not** subscribable — subscribing to one is an error; poll those or subscribe to the static parent. (`sessions` changes only on connect/disconnect; `rescan` *is* subscribable for a live activity panel.) Updates are **coalesced**: a burst of changes inside a per-resource debounce window collapses to one notification on the trailing edge (`logs`/`temperature` never flood). The windows are config-driven (`server.resources` in `.remindb/config.json`; see [configuration.md](https://github.com/radimsem/remindb/blob/main/docs/configuration.md)). The notification carries only the URI — re-read the resource to get the new state. `resources/list_changed` is never sent: the resource set is fixed for the process lifetime.

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
