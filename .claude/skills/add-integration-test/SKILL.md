---
name: add-integration-test
description: Use when adding an end-to-end test scenario to remindb — symptoms include "test the full pipeline", "simulate an agent session", "verify compile→search→fetch flow", "test MCP tools end-to-end", or any task that creates a new `Test*` in `integration_test.go` / `mcp_integration_test.go` at the repo root. Distinct from per-package unit tests in `pkg/*/`.
---

# Add an integration test

Integration tests live at the repo root in `package remindb_test`, not inside a `pkg/`. They exercise the full pipeline — a real store, real compiler, real MCP transport — against fixtures under `testdata/`. There are two flavors:

| Flavor | File | Use when |
|---|---|---|
| Direct API | `integration_test.go` | Testing compiler / query / store / transformer end-to-end without MCP |
| MCP | `mcp_integration_test.go` | Testing the user-visible tool surface as an agent would call it |

Pick the flavor by the question you're asking. "Does the compiler emit the right shape from this fixture?" → direct. "Does an agent calling `MemorySearch` after `MemoryCompile` see the expected ranking?" → MCP.

## Where it lands

| File | What changes |
|---|---|
| `integration_test.go` *or* `mcp_integration_test.go` | Add a `Test<Scenario>` function |
| `testdata/<scenario-name>/` | New fixture directory (only if existing fixtures don't match) |

Fixture directories are flat namespaces (`testdata/openclaw/`, `testdata/<your-scenario>/`). One fixture set per scenario family; reuse aggressively.

## The two helpers

Both files lean on package helpers — never reach for raw `store.Open` or raw `mcp.NewServer` in a test.

- `internal/testutil.OpenTestDB(t)` — opens an in-memory `:memory:` SQLite store with migrations applied; auto-cleans on `t.Cleanup`. Use for direct-API tests.
- `internal/mcptest.NewEnv(t)` — opens the test DB, wires a default-config server + tracker, connects an in-memory MCP transport, and exposes `env.CallTool(t, name, args)` and `env.TextContent(t, result)`. Use for MCP tests.

## Direct-API template

Mirror `integration_test.go:TestOpenClawAgentWorkflow`. Walk a real workflow stage by stage, log progress, assert at each stage:

```go
package remindb_test

import (
    "context"
    "strings"
    "testing"

    "github.com/radimsem/remindb/internal/testutil"
    "github.com/radimsem/remindb/pkg/compiler"
    "github.com/radimsem/remindb/pkg/query"
)

func TestNewScenario(t *testing.T) {
    st := testutil.OpenTestDB(t)
    ctx := context.Background()

    // Stage 1: compile fixtures.
    result, err := compiler.CompileDir(ctx, st, "testdata/<scenario>", "<source-id>")
    if err != nil {
        t.Fatalf("CompileDir: %v", err)
    }
    if result.Added == 0 {
        t.Fatal("expected nodes from fixtures")
    }
    t.Logf("compiled: +%d ~%d -%d (%d total)", result.Added, result.Modified, result.Removed, result.Total)

    // Stage 2: query.
    engine := query.NewEngine(st)
    res, err := engine.Search(ctx, "<query>", 1000)
    if err != nil {
        t.Fatalf("Search: %v", err)
    }
    if len(res.Nodes) == 0 {
        t.Fatal("expected search hits")
    }
}
```

## MCP template

Mirror `mcp_integration_test.go:TestMcp_OpenClawAgent`. The numbered-step structure is convention — each step represents one tool call an agent would make:

```go
func TestMcp_NewScenario(t *testing.T) {
    env := mcptest.NewEnv(t)
    dir, _ := filepath.Abs("testdata/<scenario>")

    // 1. Compile.
    compileResult := env.CallTool(t, "MemoryCompile", map[string]any{
        "path":    dir,
        "message": "<scenario>-init",
    })
    text := env.TextContent(t, compileResult)
    if !strings.Contains(text, "compiled") {
        t.Fatalf("unexpected compile result: %s", text)
    }

    // 2. Search.
    searchResult := env.CallTool(t, "MemorySearch", map[string]any{
        "query":  "<keyword list>",
        "budget": 1000,
    })
    searchText := env.TextContent(t, searchResult)
    if searchText == "no results" {
        t.Fatal("expected results")
    }

    // 3. Further stages...
}
```

`env.CallTool` already calls `t.Fatalf` on transport / tool errors and logs both request and response preview to `t.Log`. You only assert the *behavioral* expectations.

## Cleanup is automatic

`testutil.OpenTestDB(t)` and `mcptest.NewEnv(t)` both register `t.Cleanup` handlers — DB is dropped, MCP session is closed, in-memory transports are torn down. Don't write your own cleanup unless you allocate something outside these helpers (e.g., a `t.TempDir()` you wrote files into — that's also auto-cleaned by the testing framework, no manual work needed).

## Fixtures under `testdata/`

`testdata/` is the Go-recognized name; `go test` excludes it from build/lint. Place a fixture directory under it per scenario, with the same file extensions the parser recognizes (`.md`, `.yaml`, `.json`, `.jsonl`, `.toon`).

Name fixtures by *concept* (`testdata/openclaw/`, `testdata/multi-format/`), not by *test* (`testdata/test_compile_1/`). Multiple tests can share a fixture; one-test-per-fixture is anti-pattern.

## Quick reference

```
Direct API:
1. integration_test.go              (Test<Scenario> with testutil.OpenTestDB)
2. testdata/<scenario>/             (fixtures matching parser-supported extensions)
3. go test ./... -run Test<Scenario>

MCP:
1. mcp_integration_test.go          (TestMcp_<Scenario> with mcptest.NewEnv)
2. testdata/<scenario>/             (same fixture rules)
3. go test ./... -run TestMcp_<Scenario>
```

## Common mistakes

- **Reaching for `store.Open` directly.** Use `testutil.OpenTestDB(t)` — it applies migrations, registers cleanup, and matches the assertion style of every other test. Direct `store.Open` skips migrations and gives you an empty schema.
- **Setting up the MCP transport by hand.** `pkg/mcp/server_test.go` does this for unit-testing the server itself; integration tests should use `mcptest.NewEnv(t)`. The handcrafted form has 6 lines of ceremony (`NewInMemoryTransports`, `srv.Connect`, `mcp.NewClient`, `client.Connect`, `t.Cleanup` for both, `SetLoggingLevel` if needed) — `NewEnv` collapses all of it.
- **Asserting on log output.** `env.CallTool` logs request/response previews via `t.Log` for debugging. They're not part of the test contract; assert on `env.TextContent(t, result)` instead.
- **One fixture per test.** Reuse `testdata/openclaw/` for any scenario where its files are appropriate. New fixture directories are for genuinely new content shapes, not new test names.
- **Hardcoding the absolute path to `testdata/`.** Use `filepath.Abs("testdata/<scenario>")` like `mcp_integration_test.go:14` — `MemoryCompile` requires absolute paths.
- **Forgetting `package remindb_test`.** Integration tests are external (use `_test` suffix on the package). They exercise the public API of multiple packages — they're not allowed to import unexported names.

## Cross-references

- `.claude/rules/go-concise.md` — early returns, error handling style, named function over closure-as-var
- `.claude/skills/add-mcp-tool/SKILL.md` — when adding a tool, also extend `mcp_integration_test.go` with a scenario that exercises it through `env.CallTool`
- `.claude/skills/add-store-query/SKILL.md` — when adding a query, the unit test goes in `pkg/store/store_test.go`; an integration test only makes sense if it changes a multi-package workflow
