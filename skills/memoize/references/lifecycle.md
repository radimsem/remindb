# Lifecycle — forget, rollback, pin, summarize, recompile

Reference for `memoize`. Load when removing/reverting nodes, protecting them from decay, compacting cold nodes, re-syncing from disk, or deciding *when* to do each.

## Maintenance cadence — when to reach for each

The write tools fall into a rhythm. Use this to decide which to run, not just how:

- **On every "remember this"** → `MemoryWrite` (search-first). The default.
- **On a `remindb.temperature` warning** → `MemorySummarize` the listed nodes. The notification is the trigger; don't summarize proactively unless a node is genuinely stale.
- **When source files changed on disk** (external edit, `git pull`, disabled watcher) → `MemoryCompile` the narrow path. The background rescan usually handles this; compile manually only when you can't wait for the next tick or rescan is off.
- **When a node must never cool** (invariant, canonical summary) → `MemoryPin`, sparingly. Over-pinning kills the cold-set signal.
- **When a node is wrong / stale / never belonged** → `MemoryForget` (one node) — *not* an empty overwrite.
- **When several recent writes left the graph bad** → `MemoryRollback` to a known-good snapshot. One bad node → `MemoryForget` instead (smaller blast radius).

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

**Atomicity:** the whole flow (mutations, snapshot, diffs, cursor advance, optional prune) runs in one `Store.Tx` — a crash/cancel rolls back cleanly. This is why `MemoryRollback` bypasses `emitter.Emit` (two transactions for emit + prune would leave a real failure window). Choose rollback over `MemoryForget` for multiple bad writes / a polluted compile / privacy-sensitive content (`drop_after=true`); use `MemoryForget` for a single bad node.

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

Same shape rules apply to the `summary` — give it headings/a list; a dense paragraph is what you're compacting *away from*. The summary should index *better* than the original, not just be shorter. Notifications dedup per `ColdNotifyTTL` (default 1 hour); the next reminder only arrives if the node decays back below `ColdThreshold`. When a deployment sets `temperature.enabled: false` the ticker is frozen — no cold notifications fire, so this handoff simply never triggers until temperature is re-enabled (you can still summarize proactively).

## Recompile when the source drifts

`MemoryCompile` snapshots — same write semantics as `MemoryWrite`.

```
remindb__MemoryCompile(path="<file or subdir>", message="<optional snapshot note>")
```

Use when disk changed outside the rescan loop (external edit, disabled watcher, fresh `git pull`). **Prefer narrow paths** — one file is milliseconds; the whole tree is slow and creates a large snapshot. `path` may be absolute or relative; the server re-anchors it to `REMINDB_SOURCE` so the form you pass doesn't fork duplicate nodes (paths outside the root, or with `REMINDB_SOURCE` unset, pass through).

A `.remindb/ignore` at the source root is honored by compile + rescan — gitignore-style subset (literals, `*`/`?`/`[abc]`, trailing `/` dir-only, leading `/` root-anchor, `**` any-segment, `!` negation last-match-wins, `\` escape, `#` comments). Patterns subtract from the supported-extension allow-list; they can't re-include hardcoded skip dirs (`node_modules`, `vendor`, `target`, `dist`, `venv`) or dotfiles. Operators set this once — the agent doesn't author it.

A sibling `.remindb/pinned` (same grammar) pre-seeds the `pinned` column on every node from a matched file at insert time — useful for files an operator wants permanently warm (`README.md`, `**/CONTEXT.md`, security policies). Composes with `.remindb/temperatures.json`: `pinned` says **whether**, `temperatures.json` says **at what temperature**. Existing nodes' pin column is never touched on recompile, so `MemoryPin`/`MemoryUnpin` choices survive. The CLI-only `--reseed-pinned` flag (which forces re-pinning of manually-unpinned nodes) is **not** exposed via `MemoryCompile` — by design, the agent can't reseed its own pin signal.
