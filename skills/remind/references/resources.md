# Resources — passive read-only views

Reference for `remind`. Load when a renderer (desktop client, dashboard) needs the locked JSON envelopes, or when you must read state **without** warming nodes.

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

`remindb://temperature` — the heatmap view: every node in one `nodes` array (hot, cold, pinned all together — the renderer classifies from `temperature` vs the echoed cut points), plus an aggregate `summary`. Both thresholds are sourced from the **live configured** values (`.remindb/config.json` → `temperature.cold_threshold` / `temperature.hot_threshold`), not hardcoded constants:

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

`status` is the worst check status across the report (`fail` beats `warn` beats `pass`); `checks` is always present and ordered. It runs the same checks the `remindb doctor` CLI does — no duplicated logic — and, like every resource, warms nothing.

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

## Subscriptions: live updates instead of polling

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
