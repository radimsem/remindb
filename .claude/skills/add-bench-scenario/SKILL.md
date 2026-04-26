---
name: add-bench-scenario
description: Use when adding a new scenario to remindb's benchmark suite — symptoms include "compare X tool against grep/cat", "add a token-savings benchmark for Y", "extend `internal/bench/scenarios.go`", "wire a new scenario into `bench.Run`", or any task that adds a row to the `scenario / naive (tok) / remindb (tok) / saved` output table. Distinct from Go `testing.B` benchmarks in `*_bench_test.go`.
---

# Add a benchmark scenario

`internal/bench/` is remindb's external-facing benchmark — it compares "how many tokens does an agent consume to do X via remindb tools" against "how many would they consume doing X with `grep` / `cat` / `find`". The output is a token-savings table rendered via `text/tabwriter`. It's invoked from the `remindb bench` CLI subcommand and exercised end-to-end by `scripts/bench-agents.sh`.

This is *not* the Go `testing.B` benchmark surface — those live as `Benchmark*` functions inside `pkg/*/bench_test.go` and have their own discipline.

## Where it lands

Two files for a typical scenario, three if it needs CLI flag plumbing.

| File | What changes |
|---|---|
| `internal/bench/scenarios.go` | New `benchXxx(ctx, session, srcDir, ...) (scenarioResult, error)` function |
| `internal/bench/bench.go` | Wire the new scenario into `Run`, append to `results` |
| `cmd/remindb/...` (only if new flag needed) | Surface a new flag on the `bench` subcommand and pass it into `bench.Config` |

## The scenario function shape

Every scenario implements the same contract: produce one (or more) `scenarioResult{name, naiveTok, remindbTok}` by measuring **two paths to the same answer** — the naive path (token count of what an agent would have to read using shell tools) and the remindb path (token count of the tool's response).

Mirror `benchTree`, `benchSearch`, or `benchFetch` in `scenarios.go`:

```go
func benchExample(ctx context.Context, s *gomcp.ClientSession, srcDir string, budget int) (scenarioResult, error) {
    // Naive path: what the agent would read using shell tools.
    naive := tokens.Estimate(naiveContent(srcDir))

    // remindb path: call the tool and count its response.
    text, err := callTool(ctx, s, "MemoryExample", map[string]any{
        "budget": budget,
    })
    if err != nil {
        return scenarioResult{}, err
    }

    return scenarioResult{
        name:       "example",
        naiveTok:   naive,
        remindbTok: tokens.Estimate(text),
    }, nil
}
```

A scenario that runs over multiple inputs (like `benchSearch` over a query list) returns `[]scenarioResult` instead — one row per input, named with the input as suffix (`"search:rate-limit"`).

Three contracts the function must honor:

1. **Use `tokens.Estimate(string)`** for both sides. Apples-to-apples comparison only works if the same tokenizer measures both.
2. **Compare to a *believable* naive baseline.** "What would the agent actually do without remindb?" — `find` + `cat *` for tree (not `wc -c`), `grep` + `cat <matches>` for search (not just `grep`). The naive must reflect the real alternative.
3. **Use `callTool(ctx, s, name, args)`** (the package helper in `scenarios.go`), not raw `s.CallTool`. It strips the `*mcp.CallToolResult` boilerplate and returns the text content directly.

## Wiring into `Run`

`bench.go:Run` calls each scenario in sequence and appends to `results`. Add your call in the natural sequence — Tree first (orientation), then Search, then Fetch, then Delta, then your scenario:

```go
r, err = benchExample(ctx, session, stage.srcDir, cfg.Budget)
if err != nil {
    return err
}
results = append(results, r)
```

Or for a multi-result scenario:

```go
rs, err := benchExample(ctx, session, stage.srcDir, cfg.Budget)
if err != nil {
    return err
}
results = append(results, rs...)
```

The renderer handles the rest — the table column count and total-row computation are static.

## Naming rows

The `name` field becomes the leftmost column. Use lowercase, hyphenated, prefixed by the tool category if the scenario family has multiple rows:

- `tree` (single)
- `search:rate-limit` (one of many search queries)
- `fetch` (single)
- `delta` (single)

Stay under ~30 chars; `shorten(s, 30)` is the existing helper used by `benchSearch`.

## When to add a new flag

Most scenarios reuse `cfg.Budget` and `cfg.Queries`. Add a new field to `bench.Config` only when the scenario needs an input that isn't already there — and even then, default it sensibly so existing `bench-agents.sh` runs don't break.

## Quick reference

```
1. internal/bench/scenarios.go        (benchXxx function with naive + remindb paths)
2. internal/bench/bench.go            (call site in Run, append to results)
3. (optional) cmd/remindb/...         (new flag on the bench subcommand)
4. go build ./... && ./remindb bench --db <path> --dir <path>
5. scripts/bench-agents.sh            (full suite — confirms the new row appears for every agent)
```

## Common mistakes

- **Comparing to a trivial naive baseline.** "remindb tree returns 200 tokens, naive `ls` returns 50 tokens — we're 4x worse" misses the point. The naive is *what the agent actually does*: `find . -type f` + reading every file. Use the existing helpers (`listDirFiles`, `countDirTokens`, `grepDir`, `sumFileTokens`).
- **Using a different tokenizer for one side.** Both sides must go through `tokens.Estimate`. Counting bytes, lines, or words on one side breaks the comparison.
- **Forgetting `tokens.Estimate` is over a `string`, not `[]byte`.** Convert byte slices first; mixing produces a confusing compile error since `tokens.Estimate` is overloaded by neither Go nor remindb.
- **Failing the entire bench on one scenario error.** The current contract is fail-fast — `Run` returns the first error. If your scenario has expected partial-failure modes (skipped queries, missing fixtures), handle them inside the scenario and emit a `scenarioResult{name: "example:skipped", ...}` instead of returning err.
- **Running benches against the live DB.** `stageBench` copies the DB and source tree to `/tmp` before running so the user's real DB isn't touched. If you reach for `cfg.DBPath` directly inside a scenario, you're operating on the original — pass `stage.dbPath` instead.
- **Not updating `bench-agents.sh` query lists.** New scenarios that take per-agent inputs won't be exercised by the CI-style script unless the per-agent map gets entries. For non-input scenarios (like a new fetch variant), no script change is needed.

## Cross-references

- `.claude/rules/go-concise.md` — error wrapping, named locals (`naive`, `remindbTok`)
- `.claude/skills/add-mcp-tool/SKILL.md` — when adding a tool that should be benchmarked, do that skill first; the bench scenario consumes the tool you wired
- `internal/bench/render.go` — the table renderer; rarely needs changes but read it once if you're adding a column rather than a row
