# CLAUDE.md

Routing map for Claude. Points at the rule, skill, agent, or file that owns the answer — does not restate them.

## Project at a glance

`remindb` — token-efficient agentic memory database. Single SQLite file, MCP server on top. Go 1.26+. Pre-1.0 beta, solo dev, no compatibility shims. **`dev` is the integration trunk; `main` is a release-marker branch — one squash commit per published release, signed by GitHub's web-flow key via PR from `dev`.** Topic branches (`feat/`, `fix/`, `chore/`, `docs/`) off `dev`, rc tags on `dev`, stable cuts tag `dev` then squash-PR `dev` → `main`; lazy `release/vX.Y` branches handle patches to non-current minors; subject-only signed commits everywhere (release squash on `main` carries the release notes as its body) — see `.claude/rules/git-versioning.md`.

Pipeline: `parser → transformer → emitter → store`. Read side: `query → mcp/tools`. Background: `temperature` ticker decays/notifies.

## Code map

- `pkg/parser/` — file formats → AST nodes
- `pkg/transformer/` — AST → `ContextNode` tree
- `pkg/emitter/` — node tree → store snapshot
- `pkg/store/` — SQLite layer; `queries.go` holds SQL constants, methods live in `node.go` / `snapshot.go` / `search.go` / `temperature.go`
- `pkg/diff/` — snapshot diffing
- `pkg/query/` — search/fetch engine + result formatters (`Format`, `FormatCompact`)
- `pkg/compiler/` — full-workspace compile pipeline
- `pkg/mcp/` — MCP server; tools in `pkg/mcp/tools/` (one file per tool)
- `pkg/temperature/` — decay/boost/cold-set + cold-node notifier
- `cmd/remindb/` — CLI: `serve`, `compile`, `inspect`, `bench`, `doctor`, `update`
- `migrations/` — `0001_init.sql`, `0002_*.sql`, applied via embed.FS in `migrations.go`
- `internal/` — bench, contentid, fileext, ignore, mcptest, tempfile, testutil, tokens
- `skills/remind/`, `skills/memoize/` — **public** skills shipped to MCP clients: `remind` is the read path + mental model, `memoize` is the write path + Markdown-shape rules (distinct from `.claude/skills/`)
- `docs/` — **public** end-user manual: `architecture.md` / `cli.md` / `configuration.md` (reference) plus one deep-dive per feature; editable diagram sources in `assets/excalidraw/`, exported `assets/*.svg`
- `plugins/` — per-agent plugin folders (claude-code, gemini-cli, codex, opencode, openclaw)
- Top-level: `integration_test.go`, `mcp_integration_test.go`, `bench_test.go`

## Where to read first — don't grep, don't ls

| Question | Source |
|---|---|
| End-to-end product story, benchmarks, feature pitch | `README.md` |
| Architecture, CLI, `.remindb/` config (reference) | `docs/architecture.md` · `docs/cli.md` · `docs/configuration.md` |
| Feature deep-dive — node tree · temperature · versioning · search · TOON · MathML→LaTeX · knowledge graph | the matching `docs/<topic>.md` |
| How clients call the MCP read tools (the contract) | `skills/remind/SKILL.md` |
| How clients author content for MCP write tools (the contract) | `skills/memoize/SKILL.md` |
| Go style, naming, error/log/concurrency idioms | `.claude/rules/go-concise.md` |
| Commit, sign, branch, tag, push, release rules | `.claude/rules/git-versioning.md` |
| MCP tool contract (signature, locking, returns) | `.claude/rules/mcp-tool-conventions.md` |
| `slog` levels, fields, what never logs | `.claude/rules/logging-conventions.md` |
| User & feedback memory across sessions | `.claude/projects/-home-radimsem-personal-projects-remindb/memory/MEMORY.md` (auto-loaded) |

## Tread carefully — pause before implementing

These zones have either an external contract or a silent-drift hazard. Don't change them blind; the linked skill or rule encodes the discipline that prevents the foot-gun.

### MCP tool surface (`pkg/mcp/tools/`, `pkg/mcp/server.go`)

The `Memory*` tool suite is a contract shipped to clients via two public skills: `skills/remind/SKILL.md` (read tools — `MemoryTree`, `MemorySearch`, `MemoryFetch`, `MemoryFetchBatch`, `MemoryDelta`, `MemoryDiff`, `MemoryHistory`, `MemoryRelated`, `MemoryStats`) and `skills/memoize/SKILL.md` (write tools — `MemoryWrite`, `MemorySummarize`, `MemoryCompile`, `MemoryRelate`, `MemoryForget`, `MemoryRollback`, `MemoryPin`, `MemoryUnpin`). Renaming, removing, or changing semantics breaks every client and desyncs the relevant public skill. Use the **`add-mcp-tool` skill** for any new/modified tool, follow `.claude/rules/mcp-tool-conventions.md`, and dispatch the **`mcp-surface-reviewer` agent** before merge.

### SQLite schema & migrations (`migrations/`, `pkg/store/`)

Migrations are forward-only, applied at startup, and FTS5 triggers must stay in sync with the `nodes` table. Schema mistakes ship as broken `.db` files in user repos. Use the **`add-store-query` skill** (covers both query-only and migration-bearing changes) and dispatch the **`migration-safety-reviewer` agent** before merge.

### Temperature policy (`pkg/temperature/Config`)

`DecayRate`, `AccessBoost`, `ColdThreshold`, `NotifyThreshold`, `TickInterval` are documented numerically in `skills/remind/SKILL.md` (mental model) and the summarization workflow they trigger lives in `skills/memoize/SKILL.md`. Changing any one shifts search ranking, the cold-set query, *and* the client notification stream — both public skills drift silently. Use the **`tune-temperature-policy` skill**.

### Snapshot atomicity

Each `MemoryWrite` / `MemorySummarize` / `MemoryCompile` call must produce **exactly one** `emitter.Emit` (one snapshot row). Two snapshots per intent fragments the diff trail clients walk via `MemoryDelta`. `MemoryRelate` is the deliberate exception — relation mutations are a sideband and **must not** call `emitter.Emit`. Detail in `.claude/rules/mcp-tool-conventions.md` §7.

### Read vs. write tool discipline

Read tools (`Search`, `Fetch`, `Tree`, `Delta`, `History`, `Related`) **never** take `Store.OpMu` and **always** call `boostResultNodes` (or its equivalent — `MemoryRelated` uses `boostRelatedNodes`). Write tools (`Write`, `Summarize`, `Compile`, `Relate`) **always** take `Store.OpMu` and **never** boost. Mixing breaks ranking or serializes reads. Detail in `.claude/rules/mcp-tool-conventions.md` §5–6.

### Sync primitives

`Store.OpMu sync.Mutex` is exposed as a field. Call `.Lock()` / `.Unlock()` directly. **Do not** wrap in `LockOp` / `UnlockOp` helpers — explicit project preference (see auto-memory `feedback_sync_primitives.md`).

### Pre-1.0 beta — no compatibility shims

Solo, linear `dev`, small surface area, no committed-to-stable-API yet. Don't add deprecation paths, version gates, or backwards-compat wrappers. Just change the code and update the rule/skill that documents it.

## Workflow shortcuts — task → skill

| Task | Skill |
|---|---|
| Add a new file format to the parser | `add-parser` |
| Add or change an MCP `Memory*` tool | `add-mcp-tool` |
| Add a SQL query, column, index, or migration | `add-store-query` |
| Add a `Fuzz*` target or extend a seed corpus | `add-fuzz-target` |
| Add an end-to-end scenario (`integration_test.go`, `mcp_integration_test.go`) | `add-integration-test` |
| Add a token-savings benchmark scenario | `add-bench-scenario` |
| Tune decay / cold / notify thresholds | `tune-temperature-policy` |

Reviewer agents (`.claude/agents/`): `go-style-reviewer`, `mcp-surface-reviewer`, `migration-safety-reviewer` — dispatch on relevant changes before commit.

## Build & test

```
make build          # go build ./...
make test           # go test ./...
make test-all       # scripts/test.sh — full suite incl. integration
make fuzz           # scripts/fuzz.sh — bounded fuzz pass
make fmt lint tidy  # gofmt / golangci-lint / go mod tidy
```

Inspect a compiled DB: `go run ./cmd/remindb inspect <path>`. Run the server: `go run ./cmd/remindb serve` (add `--verbose` for `Debug` logs).

Benchmarks: `scripts/bench-agents.sh` runs the cross-agent token-savings table referenced in the README.

## What this file is NOT

- **Not** a Go style guide → `.claude/rules/go-concise.md` owns that.
- **Not** the MCP contract spec → `.claude/rules/mcp-tool-conventions.md` + `skills/remind/SKILL.md` (read) + `skills/memoize/SKILL.md` (write).
- **Not** a how-to for adding parsers/tools/queries → the `.claude/skills/` workflow each owns its own checklist.
- **Not** a changelog or task tracker → use commits and the conversation, not this file.

If a section here grows beyond a routing pointer, move the detail into the appropriate rule or skill and shrink this back to a one-liner.
