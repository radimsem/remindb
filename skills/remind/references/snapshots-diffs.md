# Snapshots, diffs, and history

Reference for `remind`'s resync/compare/history tools (`MemoryDelta`, `MemoryDiff`, `MemoryHistory`). Load when you need the exact mechanics of comparing two points in time.

## The model

### Snapshots

Every `MemoryCompile`/`MemoryWrite` creates a **snapshot**: an auto-increment `id` (int64) + an opaque `cursor_hash` (xxhash64), unique per snapshot. Linear parent chain. Pass the **id** to `MemoryDelta`; the **hash** is for equality comparison only — not a stable content digest (the same content state may hash differently across snapshots), and the two are not interchangeable.

### Diffs

Each snapshot carries per-node diffs (`add`/`mod`/`rem`, old+new content preserved). `MemoryDelta` = diffs since a known snapshot (upper bound always HEAD); `MemoryDiff` = state-vs-state between two arbitrary snapshots (git-diff hunks); `MemoryHistory` = the diff trail for one node.

**Structural-only mods:** a `mod` with `old_hash == new_hash` means content unchanged but tree position moved. Main producer: `MemoryForget mode=reparent` (children rewired to the deleted node's parent — each shows as a content-identical mod alongside the target's `rem`). Seeing `old_content == new_content`? Look for a same-snapshot `rem`.

## Resync and compare: MemoryDelta / MemoryDiff

Picked by which end of the range is fixed.

**`MemoryDelta`** — "what changed since X?", upper bound always HEAD. Use on resume / after external writes; pass the last snapshot **id** seen:

```
remindb__MemoryDelta(since_snapshot=42)             # snapshot ID (int64), not cursor_hash
remindb__MemoryDelta(since_snapshot=0)              # all changes ever, bounded by limit
remindb__MemoryDelta(since_snapshot=42, limit=1000) # raise the cap (default 500, max 5000)
```

Returns `[op] node_id (snapshot N)` lines; fetch nodes you need. Keep the last snapshot id from a prior tree/search/write result.

**`limit`** caps rows (default 500, ceiling 5000 — over-asks clamp, not error). When more changes exist the result ends with a `note:` line — **follow it, don't assume you're synced**:

- `note: truncated at limit N; ... Re-call MemoryDelta(since_snapshot=M)` → the cap landed on a snapshot boundary; resume from `M` (the last *complete* snapshot, no rows skipped).
- `note: snapshot M alone has more than N diffs; raise limit (max 5000)` → one snapshot is bigger than the current cap, so advancing would skip its tail; bump `limit` instead.
- `note: snapshot M exceeds the max page of 5000 diffs ... read it in full with MemoryDiff(...)` → a single snapshot is bigger than the hard ceiling; `MemoryDelta` can't page within one snapshot, so the note hands you the exact `MemoryDiff` call (that snapshot is the immediate next one, so the diff is scoped to it alone).

Truncation is signalled by the **note's presence**, not the row count (a trimmed page can return fewer than `limit` rows).

**`MemoryDiff`** — "what changed between X and Y?", both ends fixed. Like `git diff X Y`: compares state-at-X vs state-at-Y, not the event log between. Lower bound exclusive, upper inclusive:

```
remindb__MemoryDiff(from_snapshot_id=40, to_snapshot_id=42)   # state(40) → state(42)
```

One git-diff block per **changed node**; intermediate jitter (`mod→mod→mod`, `add→mod`) collapses to the net change. Block = `[op] node_id` + unified-diff hunks (`@@`, `-`/`+`/context). Nodes ending where they started are dropped silently (like `git diff X Y`). `from > to` → validation error; `from == to` → `no changes`.

## Inspect history before rewriting

Before a `memorize`-side overwrite, check how a node evolved:

```
remindb__MemoryHistory(anchor="<node_id>", depth=10)
```

Snapshot-ordered `add`/`mod`/`rem` with truncated old+new content. Use it to roll back (re-write the `old` payload via `memorize`'s `MemoryWrite`) or cite prior wording.
