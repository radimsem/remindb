# MCP Tool Conventions

Rules for designing and modifying tools exposed via the MCP server in `remindb`.

**Use when:** writing or reviewing anything in `pkg/mcp/tools/`, `pkg/mcp/server.go`, or any code that adds, modifies, or wraps an MCP tool entry point.

**Scope:** the *contract* of an MCP tool — what shape it takes, what it returns, what it touches, what it logs. The workflow of *adding* a new tool lives in `.claude/skills/add-mcp-tool/SKILL.md`; this file is the always-on guardrail for any change in the tool layer.

**Priority when rules conflict:** correctness > stable client contract > log signal > brevity.

---

## 1. Tools Are `Memory<Verb>`, Always ★

Every public MCP tool name is `Memory` + a single verb in PascalCase. The current set is `MemoryTree`, `MemorySearch`, `MemoryFetch`, `MemoryFetchBatch`, `MemoryDelta`, `MemoryDiff`, `MemoryHistory`, `MemoryRelated`, `MemoryStats`, `MemoryWrite`, `MemorySummarize`, `MemoryCompile`, `MemoryRelate`, `MemoryForget`, `MemoryRollback`, `MemoryPin`, `MemoryUnpin` — the canonical list is whatever `pkg/mcp/` registers; treat any count here as derived, not authoritative. Anything new follows the same shape.

```go
// Bad — non-prefixed name; clients can't filter the tool list
mcp.AddTool(srv, &mcp.Tool{Name: "Search", ...}, d.HandleSearch)

// Bad — multi-word verb, plural, or noun
mcp.AddTool(srv, &mcp.Tool{Name: "MemoryFindAndSort", ...}, ...)
mcp.AddTool(srv, &mcp.Tool{Name: "MemoryAccessLogs", ...}, ...)

// Good
mcp.AddTool(srv, &mcp.Tool{Name: "MemorySearch", ...}, d.HandleSearch)
mcp.AddTool(srv, &mcp.Tool{Name: "MemorySummarize", ...}, d.HandleSummarize)
```

The same string passed to `mcp.AddTool` must be passed to `defer d.logCall("MemoryX", ...)`. Mismatch breaks per-tool log filtering.

---

## 2. The Handler Signature Is Fixed ★

Every tool is a method on `*Deps` with the named-return signature the SDK expects. The `err` name is **mandatory** — `defer d.logCall(..., &err, ...)` captures it by pointer.

```go
// Bad — anonymous error return; deferred logger captures nil
func (d *Deps) HandleX(ctx context.Context, _ *gomcp.CallToolRequest, input XInput) (*gomcp.CallToolResult, any, error) {
    defer d.logCall("MemoryX", nil, time.Now())   // can't capture err
    ...
}

// Bad — shadowing err with `:=` in the body
func (d *Deps) HandleX(ctx context.Context, _ *gomcp.CallToolRequest, input XInput) (_ *gomcp.CallToolResult, _ any, err error) {
    defer d.logCall("MemoryX", &err, time.Now())
    result, err := doWork()    // OK
    if other, err := nestedCall(); err != nil {   // BAD — shadows; defer sees the outer nil
        return nil, nil, err
    }
    ...
}

// Good
func (d *Deps) HandleX(ctx context.Context, _ *gomcp.CallToolRequest, input XInput) (_ *gomcp.CallToolResult, _ any, err error) {
    defer d.logCall("MemoryX", &err, time.Now(), "anchor", input.Anchor, "budget", input.Budget)
    ...
}
```

The middle return value (`_ any`) is the SDK's structured-output slot. We don't use it — every tool returns text content (see §4).

---

## 3. Input Is a Typed Struct With `jsonschema` Tags ★

Tool inputs are top-of-file structs named `<Verb>Input`, with `json` and `jsonschema` tags on every field. The `jsonschema` tag is the description the client (and any IDE that reads tool schemas) sees.

```go
// Bad — untyped map; clients have no idea what to send
func (d *Deps) HandleSearch(ctx context.Context, _ *gomcp.CallToolRequest, input map[string]any) (...) { ... }

// Bad — no jsonschema tag; the field shows up nameless in the tool catalog
type SearchInput struct {
    Query  string `json:"query"`
    Budget int    `json:"budget"`
}

// Good
type SearchInput struct {
    Query  string `json:"query"  jsonschema:"Full-text search query"`
    Budget int    `json:"budget" jsonschema:"Maximum token budget for the response"`
}
```

Optional fields take `omitempty` on the `json` tag and a default-mentioning `jsonschema` description: `Depth int \`json:"depth,omitempty" jsonschema:"Max descendant depth (1-128, default 32); 0 uses engine default"\``.

Validate at the boundary, not inside the body. If a field is required, the SDK enforces presence via the schema; if it has range bounds, document them in the `jsonschema` description and let the engine reject out-of-range values with a wrapped error.

---

## 4. Returns Are Always Text Content ★

Every tool returns a `*mcp.CallToolResult` whose `Content` is exactly one `*mcp.TextContent`. No structured returns, no multi-part content, no empty responses.

```go
// Bad — structured return; clients render text, not JSON
return &gomcp.CallToolResult{
    StructuredContent: result,
}, nil, nil

// Bad — empty content array on the no-result path
if len(result.Nodes) == 0 {
    return &gomcp.CallToolResult{}, nil, nil
}

// Good — happy path
text := query.Format(result)
return &gomcp.CallToolResult{
    Content: []gomcp.Content{&gomcp.TextContent{Text: text}},
}, nil, nil

// Good — explicit empty-state text
return &gomcp.CallToolResult{
    Content: []gomcp.Content{&gomcp.TextContent{Text: "no results"}},
}, nil, nil
```

Use the existing formatters in `pkg/query/` (`Format`, `FormatCompact`) for query results. New tools that need a different format should add a formatter to the same package, not inline string-building.

---

## 5. Locking: Read Tools Don't, Write Tools Do ★

`*store.Store` exposes `OpMu sync.Mutex` directly (no wrapper methods). The rule is binary:

| Tool kind | Take `Store.OpMu` |
|---|---|
| Read-only — `MemorySearch`, `MemoryFetch`, `MemoryTree`, `MemoryDelta`, `MemoryHistory`, `MemoryRelated` | **No** |
| Mutating — `MemoryWrite`, `MemorySummarize`, `MemoryCompile`, `MemoryRelate`, `MemoryRollback` | **Yes**, immediately after the `defer d.logCall(...)` |

```go
// Bad — read tool taking the lock; serializes parallel reads for no reason
func (d *Deps) HandleSearch(...) (...) {
    defer d.logCall("MemorySearch", &err, time.Now(), ...)
    d.Store.OpMu.Lock()
    defer d.Store.OpMu.Unlock()
    ...
}

// Bad — write tool not taking the lock; concurrent writes can interleave snapshots
func (d *Deps) HandleWrite(...) (...) {
    defer d.logCall("MemoryWrite", &err, time.Now(), ...)
    return d.emit(...)   // race
}

// Good — write tool, lock taken second
func (d *Deps) HandleWrite(...) (...) {
    defer d.logCall("MemoryWrite", &err, time.Now(), ...)
    d.Store.OpMu.Lock()
    defer d.Store.OpMu.Unlock()
    ...
}
```

The temperature boost (`d.boostResultNodes`) is the one read-side mutation that doesn't take `OpMu` — it goes through `Tracker.RecordAccess` → `BoostTemperatureBatch`, which serializes through SQLite's WAL writer.

---

## 6. Read Tools Boost; Write Tools Don't

After producing a result, read tools call `d.boostResultNodes(ctx, result)` to bump temperature on accessed nodes. Write tools never boost — the write itself is the access, and the snapshot it creates updates `last_accessed_at` indirectly.

```go
// Bad — write tool boosting; double-counts the access and skews temperature
func (d *Deps) HandleWrite(...) (...) {
    ...
    if err := emitter.Emit(...); err != nil { return ..., err }
    d.boostResultNodes(ctx, fakeResult)   // never do this
    return ..., nil
}

// Bad — read tool not boosting; reads don't warm the node, ranking goes stale
func (d *Deps) HandleSearch(...) (...) {
    result, err := d.Engine.Search(...)
    if err != nil { return ..., err }
    text := query.FormatCompact(result)
    return &gomcp.CallToolResult{...}, nil, nil   // missing boost
}

// Good — read tool, boost between result and format
result, err := d.Engine.Search(ctx, input.Query, input.Budget)
if err != nil { return nil, nil, fmt.Errorf("failed to search: %w", err) }
d.boostResultNodes(ctx, result)
text := query.FormatCompact(result)
```

---

## 7. Snapshot-Emitting Tools Create Exactly One Snapshot

Each call to `MemoryWrite` / `MemorySummarize` / `MemoryCompile` / `MemoryRollback` produces **one** snapshot. Snapshots are how clients diff state via `MemoryDelta`; per-token or per-node mini-snapshots fragment the diff trail.

`MemoryWrite` / `MemorySummarize` / `MemoryCompile` reach the snapshot via `emitter.Emit`. `MemoryRollback` is the **inlined-tx exception**: it bypasses `emitter.Emit` so the snapshot create, diff inserts, cursor advance, and optional `PruneSnapshotsAfterTx` happen in a single `Store.Tx`. Atomicity over an emit + prune sequence cannot be achieved through two separate `emitter.Emit`-then-prune transactions without a recoverable-but-real failure window, so the rule for `MemoryRollback` is: do the writes inline, still produce exactly one snapshot row. Treat the §7 invariant as "one snapshot per call", not literally "one `emitter.Emit` per call".

The other write tools form the **snapshot-emitting-via-emitter subset**, but not the universe of write tools. `MemoryRelate` is the second deliberate counterexample: it locks `Store.OpMu` like the others (per §5) but **must not** call `emitter.Emit` *and* **must not** create a snapshot at all. Relation mutations are a sideband — `MemoryDelta` / `MemoryHistory` don't surface them. Any future write tool decides explicitly which side of these lines it falls on; tools that mutate the snapshot-tracked node graph emit, tools that mutate sideband structures (relations, future tag layers, future cursor metadata) don't.

```go
// Bad — emitting twice in one call; two snapshot rows for one user intent
emitter.Emit(ctx, d.Store, emitter.WithRoots(beforeRoots), ...)
emitter.Emit(ctx, d.Store, emitter.WithRoots(afterRoots), ...)

// Good — assemble all roots, diff once, emit once
roots := append(beforeRoots, afterRoots...)
deltas := diff.Diff(roots, prev)
emitter.Emit(ctx, d.Store,
    emitter.WithRoots(roots),
    emitter.WithDeltas(deltas),
    emitter.WithCursorHash(diff.CursorHash(roots)),
    emitter.WithMessage("write:"+nodeID),
)
```

If the operation is genuinely two distinct intents, expose two tools, not one tool that snapshots twice.

---

## 8. Errors Wrap; Don't Reformat

Action errors carry a `failed to <verb>:` prefix and wrap with `%w` so callers can `errors.Is` (see `.claude/rules/go-concise.md` §5). Validation errors carry no prefix.

```go
// Bad — string-concat loses the chain
return nil, nil, fmt.Errorf("search error: " + err.Error())

// Bad — package prefix instead of verb
return nil, nil, fmt.Errorf("query: %w", err)

// Good — verb-framed action error
return nil, nil, fmt.Errorf("failed to search: %w", err)
return nil, nil, fmt.Errorf("failed to fetch: %w", err)

// Good — pure validation, no prefix
return nil, nil, fmt.Errorf("budget must be positive, got %d", input.Budget)
```

Never `log.Fatal` or `os.Exit` from a tool. Tools return errors; the SDK turns them into `result.IsError = true` for the client.

---

## 9. Log What Helps Debug, Never the Payload ★

`defer d.logCall(...)` takes the tool name, a pointer to `err`, the start time, and a variadic list of attrs. The attrs are the entire signal you'll have when something misbehaves.

```go
// Bad — payload as an attr; slog will serialize the whole MB
defer d.logCall("MemoryWrite", &err, time.Now(), "payload", input.Payload)

// Bad — no attrs; can't tell which call failed
defer d.logCall("MemoryWrite", &err, time.Now())

// Good — IDs and counts; never bodies
defer d.logCall("MemoryWrite", &err, time.Now(), "anchor", input.Anchor, "payload_bytes", len(input.Payload))
defer d.logCall("MemorySearch", &err, time.Now(), "query", input.Query, "budget", input.Budget)
defer d.logCall("MemorySummarize", &err, time.Now(), "node_id", input.NodeID, "summary_bytes", len(input.Summary))
```

The `query` string in `MemorySearch` is the one exception to "no user content" — it's small and necessary to debug a misbehaving search. Anything large (payload, summary, full node content) goes by byte-count only. See `.claude/rules/logging-conventions.md` for the full discipline.

---

## 10. Update the Right Public Skill On Every Tool Change ★

Two public skills under `skills/` form the client contract for what tools exist and how to call them. Pick the right one (or both) when you add, rename, or change semantics of a tool — the change must land in the same commit (or the immediate follow-up — see `.claude/rules/git-versioning.md` §2).

| Tool kind | Skill to update |
|---|---|
| Read tools (`MemoryTree`, `MemorySearch`, `MemoryFetch`, `MemoryDelta`, `MemoryHistory`, `MemoryRelated`) | **`skills/remind/SKILL.md`** |
| Write tools (`MemoryWrite`, `MemorySummarize`, `MemoryCompile`, `MemoryRelate`, `MemoryRollback`) | **`skills/memoize/SKILL.md`** |
| A tool whose change crosses the boundary (e.g., new shared concept, mental-model field, threshold name) | **Both** — `remind` owns the mental model, `memoize` owns the write workflow that depends on it |

For each affected skill:

- Add or remove the tool from the frontmatter `description` list.
- Update the opening / inventory paragraph to reflect the new surface.
- Add at least one example call into the relevant pattern section.

Tool exists in code but invisible to its public skill = invisible to future Claude sessions. The skills are part of the deployed surface, not auxiliary docs.

---

## Anti-Patterns — Do Not

- Tool name without `Memory` prefix.
- Anonymous-error-return signature on a handler.
- Untyped `map[string]any` input or input without `jsonschema` tags.
- Structured return; multi-content return; empty content array.
- Read tool taking `Store.OpMu`; write tool not taking it.
- Read tool skipping `boostResultNodes`; write tool calling it.
- More than one snapshot row per tool call — whether via two `emitter.Emit` invocations or via an inlined-tx tool calling `CreateSnapshotWithParentTx` twice. The §7 invariant is "one snapshot per call", not "one `emitter.Emit` per call".
- `log.Fatal` / `os.Exit` from a tool body.
- Logging the full payload, summary text, node content, or any user-supplied body.
- Wrapping the error with `%s` instead of `%w`.
- Adding/renaming/removing a tool without updating its public skill (`skills/remind/SKILL.md` for read tools, `skills/memoize/SKILL.md` for write tools, both when the change crosses the read/write boundary).
- Wrapping `Store.OpMu` in helper methods like `LockOp` / `UnlockOp` (memory: "no wrapper methods around sync primitives").

---

## Priority When Rules Conflict

1. **Correctness** — locking, snapshot atomicity, error chain integrity.
2. **Stable client contract** — text-only returns, fixed handler signature, `Memory<Verb>` naming. Breaking these breaks clients.
3. **Log signal** — `defer d.logCall(...)` discipline, payload-free attrs.
4. **Brevity** — prefer the shortest form that doesn't hurt 1–3.
