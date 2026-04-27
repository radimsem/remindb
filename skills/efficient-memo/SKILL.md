---
description: Use when the session has access to a remindb MCP server (tools prefixed remindb__ like MemoryTree, MemorySearch, MemoryFetch, MemoryWrite, MemoryDelta, MemoryHistory, MemorySummarize, MemoryCompile). Teaches how the server is structured, the FTS5 search-query format with its automatic OR rewrite, token-budgeted search and fetch, snapshot-aware writes, delta resync that replace raw file reads and grep, and the warning-level cold-node notifications the server pushes to nudge MemorySummarize.
---

# Efficient remindb MCP usage

remindb is a compiled SQLite view of a workspace, served over MCP as eight `Memory*` tools. It's long-term memory for your session — call it instead of re-reading files or grepping.

## Mental model

### Nodes

The smallest unit of memory is a **node**. Every node has:

- An **11-char base62 ID** (e.g. `3kGXxidmWBp`), content-addressed with xxhash64. Use it as the anchor in all follow-up calls; never guess or edit it.
- A `parent_id` — nodes form a tree.
- A `label` — a short, scannable title (first meaningful line, ≤80 chars).
- A `node_type` — one of `heading`, `list`, `kv`, `table`, `preamble`, `text`, `code`. Types hint at shape, not behavior.
- A `token_count` — estimated cl100k-base tokens. The query layer uses this to honor budgets.
- A `temperature` in `[0.0, 1.0]` — how "warm" the node is. Reads boost it (`+0.15`, capped at 1.0 by SQL `min(1.0, …)`). A background tick (default every 5 minutes) decays everything by `factor = exp(-0.05 × elapsed_hours)` — roughly 5% per hour. Two thresholds gate downstream behavior, both default to `0.1`:
  - `ColdThreshold` — used by `GetColdNodes` and the search ranking floor; nodes below it are "cold".
  - `NotifyThreshold` — used by the server to decide whether to push a cold-node notification (see *Notifications* below). Distinct from `ColdThreshold` so operators can run a tighter alerting band than the cold-set query.

### Snapshots

Every `MemoryCompile` or `MemoryWrite` creates a **snapshot** — a row with an auto-increment `id` (int64) and a `cursor_hash` (xxhash64 of the whole DB state). Snapshots form a linear parent chain. The id is what you pass to `MemoryDelta`; the hash is an opaque fingerprint you can store for later comparison.

### Diffs

Every snapshot carries per-node diffs: `add`, `mod`, or `rem`, with old and new content preserved. `MemoryDelta` is how you read diffs since a known snapshot; `MemoryHistory` is how you read the diff trail for one specific node.

### Ranking

Search results are ranked by `score = relevance × (0.3 + 0.7 × temperature)`. Relevance is FTS5's BM25-like rank; temperature is the warmth above. A very cold node with a great keyword match still surfaces; a warm node with a weak match also surfaces. The budget trims results from the bottom after ranking.

### Notifications

After each tick the server pushes a cold-node nudge to every connected client session that has called `SetLoggingLevel`. The transport is the MCP `notifications/message` channel — *not* server-side stderr — with `level: "warning"` and `logger: "remindb.temperature"`. The payload is:

```json
{
  "message": "Cold nodes detected; consider summarizing via MemorySummarize",
  "suggested_action": "MemorySummarize",
  "nodes": [
    { "id": "<11-char base62>", "label": "...", "file": "...", "temperature": 0.07 }
  ]
}
```

The server dedups: a node is notified once when it crosses below `NotifyThreshold` and is then suppressed until it warms above `NotifyThreshold` and re-cools (a hysteresis band so a node oscillating around the line doesn't spam). Treat one of these notifications as a direct cue to call `MemorySummarize` on the listed `id`s — see *Summarize a cold node*.

### Budgets

Every read-side tool takes a `budget` (int, tokens). The engine fills results up to the budget and stops — cheaper than returning everything and hoping the client truncates. Reasonable defaults:

- `500` — scoped lookup, one specific fact.
- `1000` — topic exploration, a few related anchors.
- `1500` — broad sweep, discover what's in the area.

## Search-query syntax — critical

remindb's search goes through SQLite's FTS5 extension. The server does a small pre-processing step: **bare multi-word queries are rewritten to `OR` between each word**. Anything that already looks like FTS5 syntax passes through unchanged.

### How the rewrite works

Before a query hits FTS5, the server checks for any of these operators:

```
OR   AND   NOT   NEAR(   "   :   *   (
```

- If **any** of them appears in the query → **pass through** unchanged (it's already FTS5).
- Otherwise the query is whitespace-split and joined with ` OR `.
- A single bare word is passed through unchanged.

### What this means for you

```
query: "token bucket rate limit"
     → rewritten to: token OR bucket OR rate OR limit
     → matches any node containing AT LEAST ONE of those words

query: "database"
     → passed through: database
     → matches any node containing "database"

query: "token AND bucket"
     → passed through (has AND)
     → matches only nodes containing BOTH words

query: "\"token bucket\""
     → passed through (has ")
     → phrase match, matches nodes containing the exact adjacent pair
```

### How to construct queries

1. **Send keyword lists, not sentences.** Every word contributes to OR ranking. Function words ("how", "the", "do", "I", "can") dilute the ranking — strip them.
2. **Use bare multi-word queries when you want broad recall.** The OR rewrite gives you "any-of" matching with results ranked by how many words hit.
3. **Use FTS5 operators directly when you need precision.** Mix-and-match:
   - `"exact phrase"` — phrase match. Requires adjacent tokens in order.
   - `term1 AND term2` — both required.
   - `term1 NOT term2` — exclude `term2`.
   - `prefix*` — prefix match (matches `prefix`, `prefixes`, `prefixed`…).
   - `NEAR(term1 term2, 5)` — both words within 5 tokens of each other.
4. **Quote phrases with internal punctuation.** Hyphens and dots are tokenizer boundaries — search `"rate-limit"` (quoted) to match the hyphenated form.

### Examples

```
# Bad — dilutes ranking with stopwords
query: "how do I configure the rate limiter middleware"
  → rewritten: how OR do OR I OR configure OR the OR rate OR limiter OR middleware

# Good — only content keywords
query: "rate limiter middleware configure"
  → rewritten: rate OR limiter OR middleware OR configure

# Better when you know the exact phrase
query: "\"rate limiter middleware\""
  → passed through as phrase match

# When you need all three terms
query: "rate AND limiter AND redis"
  → passed through, requires all three
```

## The four core patterns

### 1. Orient: tree first, always

At session start or task switch, call `MemoryTree` once. Don't `ls`, don't `Glob`, don't `Read`.

```
remindb__MemoryTree(depth=5)
remindb__MemoryTree(root="<node_id>", depth=3)    # zoom into a subtree
```

Returns a typed, labeled hierarchy with temperatures and token counts. Scan it to pick where to look next. Temperatures tell you what has been read recently — follow hot branches first. Default depth is 5; raise it only when shallow didn't reveal the anchor you need.

### 2. Look up: MemorySearch, then MemoryFetch

Never grep. `MemorySearch` returns ranked anchors under a token budget; `MemoryFetch` expands a single anchor with its ancestors and children.

```
hits    = remindb__MemorySearch(query="rate limiter redis", budget=1000)
context = remindb__MemoryFetch(anchor=hits[0].id, budget=500, depth=32)
```

`MemoryFetch`'s `depth` controls how many levels of descendants are included (1–128, default 32). Leave at default unless you know the subtree is huge.

### 3. Persist: MemoryWrite

When the user asks you to remember something, or you discover a non-obvious fact worth keeping:

```
# Update an existing node
remindb__MemoryWrite(anchor="<node_id>", payload="<full replacement content>")

# Create a new text node
remindb__MemoryWrite(payload="<content>")   # anchor omitted → new node
```

Rules:

- `payload` is the whole content. The label is auto-derived from the first line (≤80 chars) — put the scannable title on line one.
- `anchor=""` or omitted → creates a new text node (type `text`, depth 1, source `mcp:write`). Content-addressed ID is derived from the payload.
- `anchor=<existing_id>` → replaces that node's content. Type, parent, and source are preserved.
- One logical note per call. Every write creates a snapshot.
- Prefer updating an existing node over creating a new one. Search first for a plausible anchor.

### 4. Resync: MemoryDelta

When resuming a session or after external writes, use the last snapshot `id` you saw to get only what changed:

```
remindb__MemoryDelta(since_snapshot=42)    # snapshot ID (int64), not cursor_hash
remindb__MemoryDelta(since_snapshot=0)     # all changes ever (rarely what you want)
```

Returns a list of `[op] node_id (snapshot N)` lines. Fetch the specific nodes you care about if you need content. `since_snapshot=0` returns every diff in history — expensive. Keep the last snapshot id from a prior tree/search/write result.

## Maintenance patterns

### Summarize a cold node

The server cools untouched nodes over time. When a node drops below `NotifyThreshold` it gets pushed to your session as an MCP logging notification (see *Notifications* in the mental model):

- `level: "warning"`
- `logger: "remindb.temperature"`
- `data.message: "Cold nodes detected; consider summarizing via MemorySummarize"`
- `data.suggested_action: "MemorySummarize"`
- `data.nodes: [{id, label, file, temperature}, …]`

When you see this notification, take it as the system's compaction signal — long prose that nobody is reading, condense it. For each entry in `nodes`:

```
remindb__MemoryFetch(anchor="<id>", budget=1500)        # read what's there
remindb__MemorySummarize(node_id="<id>", summary="…")   # replace with the condensed version
```

`MemorySummarize` replaces the node's content in place, recomputes `token_count`, and rewrites the label to `"Summary: <first line>"` (first line truncated to 70 chars, prefix included). Type, parent, and source file are preserved. Every call creates a snapshot, so the prior wording is recoverable via `MemoryHistory`.

Notifications are deduplicated server-side, so you won't see the same node again until it warms above `NotifyThreshold` and re-cools — there's no harm in deferring summarization, but no further reminder either.

### Recompile when the source drifts

If files on disk changed outside remindb's rescan loop (external edit, disabled watcher, fresh `git pull`):

```
remindb__MemoryCompile(path="<file or subdir>", message="<optional snapshot note>")
```

Prefer narrow paths. Compiling one file is milliseconds; compiling the entire source tree is slow and creates a large snapshot.

If a `.remindb.ignore` file lives at the source root, `MemoryCompile` (and the background rescan) honors it — same gitignore-style minimal subset (literal names, `*` wildcards, trailing `/` for dir-only, `**` for any-segment-count, `#` comments). Patterns subtract from the supported-extension allow-list; they cannot re-include `node_modules` or dotfiles. Operators set this once; the agent doesn't author the file.

### Inspect history before rewriting

Before overwriting a node, check how it evolved:

```
remindb__MemoryHistory(anchor="<node_id>", depth=10)
```

Returns snapshot-ordered `add`/`mod`/`rem` entries with truncated old and new content. Use it to roll back (re-write the `old` payload), or to cite prior wording.

## Anti-patterns — do not

- Don't `Read` / `Glob` / `Grep` source files that are already in remindb. Ask remindb.
- Don't send full sentences as queries. Keyword lists rank better under the OR rewrite.
- Don't call `MemoryTree` before every search — it's expensive. One call per orientation.
- Don't omit the budget. The server will use a default, but you lose control over cost.
- Don't write per-token or per-keystroke. Batch into one coherent `MemoryWrite`.
- Don't re-read the whole tree on resume. Use `MemoryDelta` with the last snapshot id.
- Don't edit anchor IDs — they're content-addressed. Let remindb assign them.
- Don't confuse **snapshot id** (int64, passed to `MemoryDelta`) with **cursor_hash** (xxhash64 string, a fingerprint for equality comparison only).
- Don't ignore a `level: "warning"` / `logger: "remindb.temperature"` notification. It's the server's only summarization cue and won't fire again for the same node until it warms and re-cools. Walk the `nodes` array and call `MemorySummarize` on each `id`.
- Don't confuse `ColdThreshold` with `NotifyThreshold`. Both default to `0.1`, but they're independent knobs — `ColdThreshold` drives the cold-node *query*, `NotifyThreshold` drives the *push*. A deployment can tune them separately.
