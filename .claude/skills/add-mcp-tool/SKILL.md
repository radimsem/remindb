---
name: add-mcp-tool
description: Use when adding a new `Memory*` tool to remindb's MCP server — symptoms include "expose X over MCP", "register a new tool with the SDK", "add an endpoint to pkg/mcp/tools/", "wire a new MemoryXxx into registerTools", or any task that gives MCP clients a new capability backed by the store/engine/tracker.
---

# Add a new MCP tool

Tools live in `pkg/mcp/tools/` as one file per tool. Each tool is a method on `*Deps` that takes a typed input struct, calls into `Store` / `Engine` / `Tracker`, and returns a `*mcp.CallToolResult` with text content. Adding one means *four* changes plus a docs sync.

## Where it lands

| File | What changes |
|---|---|
| `pkg/mcp/tools/<tool>.go` | New file — `XxxInput` struct + `HandleXxx` method on `*Deps` |
| `pkg/mcp/server.go` | Add a `mcp.AddTool(srv, ...)` entry to `registerTools` |
| `pkg/mcp/tools/tools_test.go` | Test using `mcptest.NewEnv` from `internal/mcptest` |
| `skills/remind/SKILL.md` *(read tools)* or `skills/memoize/SKILL.md` *(write tools)* | Add the new tool to the inventory and any pattern section it belongs in |

## Tool file template

The shape is uniform across `fetch.go`, `search.go`, `summarize.go`, `write.go`. Mirror it.

```go
package tools

import (
    "context"
    "fmt"
    "time"

    gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type ExampleInput struct {
    Anchor string `json:"anchor" jsonschema:"Node ID to operate on"`
    Budget int    `json:"budget,omitempty" jsonschema:"Token budget for the response"`
}

func (d *Deps) HandleExample(ctx context.Context, _ *gomcp.CallToolRequest, input ExampleInput) (_ *gomcp.CallToolResult, _ any, err error) {
    defer d.logCall("MemoryExample", &err, time.Now(), "anchor", input.Anchor, "budget", input.Budget)

    // Write-side tools take the lock; read-side tools do not (see "Locking" below).
    // d.Store.OpMu.Lock()
    // defer d.Store.OpMu.Unlock()

    result, err := d.Engine.DoSomething(ctx, input.Anchor, input.Budget)
    if err != nil {
        return nil, nil, fmt.Errorf("failed to do-something: %w", err)
    }

    d.boostResultNodes(ctx, result)         // read tools only

    return &gomcp.CallToolResult{
        Content: []gomcp.Content{&gomcp.TextContent{Text: result.Format()}},
    }, nil, nil
}
```

Five things every tool gets right:

1. **Named return values for the deferred logger.** `(_ *gomcp.CallToolResult, _ any, err error)` is the SDK signature; the `err` name is required so `defer d.logCall(..., &err, ...)` can capture the final error. Renaming or omitting `err` silently breaks call logging.
2. **`defer d.logCall(...)` on the first line.** Prefix the tool name with `Memory` to match the registered name. Pass enough attrs to debug a misbehaving call (anchor, budget, payload byte-count — never the full payload).
3. **Locking decision (see below).**
4. **Error wrapping.** Action errors take `failed to <verb>:` per `go-concise.md` §5; wrap the engine/store error with `%w` so callers can `errors.Is`.
5. **Return `*mcp.CallToolResult` with text content.** Format complex results into one string before returning — clients render text, not structured JSON.

## Locking

The store uses a single `sync.Mutex` exposed as `Store.OpMu` (memory: "no wrapper methods around sync primitives"). The rule:

| Tool kind | Take `OpMu` |
|---|---|
| Read-only (`MemorySearch`, `MemoryFetch`, `MemoryTree`, `MemoryDelta`, `MemoryHistory`) | **No** |
| Mutating (`MemoryWrite`, `MemorySummarize`, `MemoryCompile`) | **Yes** |

Mutating tools call `d.Store.OpMu.Lock()` immediately after the deferred logger and `defer d.Store.OpMu.Unlock()`. See `pkg/mcp/tools/summarize.go:21-22` and `write.go:24-25` for the canonical pattern.

Read tools also call `d.boostResultNodes(ctx, result)` to bump temperature on accessed nodes — mutating tools do not (the write itself is the access).

## The registration entry

Open `pkg/mcp/server.go:146-186` (the `registerTools` function) and add:

```go
mcp.AddTool(srv, &mcp.Tool{
    Name:        "MemoryExample",
    Description: "<one short sentence — what it does, not how>",
}, d.HandleExample)
```

Keep the name `Memory<Verb>` so it sorts cleanly with the existing inventory and matches the `defer d.logCall(...)` argument.

## The test

`pkg/mcp/tools/tools_test.go` uses the in-process MCP transport via `internal/mcptest.NewEnv(t)`. Call your tool through the client session and assert on the text output:

```go
func TestExample_HappyPath(t *testing.T) {
    env := mcptest.NewEnv(t)
    res := env.CallTool(t, "MemoryExample", map[string]any{
        "anchor": "<seeded-id>",
        "budget": 500,
    })
    text := env.TextContent(t, res)
    if !strings.Contains(text, "<expected substring>") {
        t.Fatalf("unexpected output: %s", text)
    }
}
```

## The docs sync — easy to skip, easy to regret

Two public skills under `skills/` are the contract with future Claude sessions about what tools exist. Pick the one that matches the tool's side:

| Tool kind | Skill to update |
|---|---|
| Read (`MemoryTree`, `MemorySearch`, `MemoryFetch`, `MemoryDelta`, `MemoryHistory`) | `skills/remind/SKILL.md` |
| Write (`MemoryWrite`, `MemorySummarize`, `MemoryCompile`) | `skills/memoize/SKILL.md` |
| Crosses the boundary (introduces a new mental-model concept used on both sides) | Both |

For each affected skill, when you add a tool:

1. Update the frontmatter `description` tool list.
2. Update the opening / inventory paragraph to reflect the new surface.
3. Add at least one example call into the relevant pattern section.

Skipping this means future sessions won't know the tool exists. The skills are the API contract, not just docs.

## Quick reference

```
1. pkg/mcp/tools/<tool>.go        (Input struct + Handle method on *Deps)
2. pkg/mcp/server.go              (mcp.AddTool entry in registerTools)
3. pkg/mcp/tools/tools_test.go    (env := mcptest.NewEnv(t); env.CallTool(...))
4. skills/remind/SKILL.md   (read tools)
   OR
   skills/memoize/SKILL.md          (write tools)
   OR both, when the change crosses the read/write boundary
5. go test ./pkg/mcp/...          (must pass)
```

## Common mistakes

- **Read tool that mutates.** If your tool mutates (even just bumping temperature), it must take `OpMu`. The temperature boost in `boostResultNodes` is the one exception — it goes through `Tracker.RecordAccess` → `BoostTemperatureBatch`, which serializes through SQLite's WAL writer, not the in-memory mutex.
- **Forgetting the named `err` return.** `defer d.logCall(..., &err, ...)` captures `err` by pointer. If the function signature uses an unnamed error or shadows `err` with `:=`, the deferred log shows `<nil>` for failed calls.
- **Returning `nil, nil, nil` on the no-result path.** Return an empty `*mcp.CallToolResult` with a text body like `"no results"` — clients expect text, not a missing content array. See `pkg/mcp/tools/search.go` and the `query.FormatCompact` "no results" string.
- **Passing the raw payload as a log attr.** Use byte-count (`"payload_bytes", len(input.Payload)`) — payloads can be MB, and `slog` will serialize the whole thing.
- **Skipping the public-skill update.** Tool exists in code but invisible to agents. Read tools must show up in `skills/remind/SKILL.md`; write tools in `skills/memoize/SKILL.md`. Test: a fresh session reading the relevant skill should be able to use the new tool from the description alone.

## Cross-references

- `.claude/rules/go-concise.md` — error wrapping, naming, locking discipline, no-wrapper-methods rule
- `.claude/rules/git-versioning.md` — one commit per logical change; the four code edits ship together as `feat(mcp): add MemoryExample tool`, the docs sync as a follow-up `docs(skill): document MemoryExample` if it grew large, otherwise bundled
- `.claude/skills/add-store-query/SKILL.md` — if the new tool needs a query the store doesn't have yet, do that skill first
- `skills/remind/SKILL.md` — docs target for **read-side** tools (`MemoryTree`, `MemorySearch`, `MemoryFetch`, `MemoryDelta`, `MemoryHistory`)
- `skills/memoize/SKILL.md` — docs target for **write-side** tools (`MemoryWrite`, `MemorySummarize`, `MemoryCompile`)
