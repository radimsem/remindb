---
project: harbor
stack: rust
runtime: opencode
default_agent: build
---

# Harbor

## Overview

Harbor is a terminal UI code search engine built in Rust. It indexes polyglot repositories with tree-sitter, stores postings in tantivy, and renders results through a ratatui interface. The target use case is navigating codebases too large for ripgrep to feel instant — multi-million-LOC monorepos where symbol-aware search beats regex scanning.

The binary runs as both a CLI (`harbor search <query>`) and a long-lived TUI (`harbor tui`). A shared `harbor-core` crate owns indexing and query planning, so the two frontends never drift in behavior.

## Project Structure

- `crates/harbor-core/` — indexing pipeline, tantivy schema, query planner; no I/O outside explicit driver traits
- `crates/harbor-cli/` — one-shot search and index commands; argument parsing via clap
- `crates/harbor-tui/` — ratatui application, event loop, key bindings; depends on `harbor-core` only
- `crates/harbor-lsp/` — optional LSP bridge that feeds symbol references back into the index
- `crates/harbor-parsers/` — tree-sitter grammar wrappers; one submodule per supported language
- `benches/` — criterion benchmarks for index build, query latency, and memory footprint
- `tests/` — integration tests that spin up a real tantivy index against fixture repos
- `xtask/` — cargo-xtask automation: `cargo xtask release`, `cargo xtask bench-diff`, `cargo xtask coverage`

## Supported Languages

Tree-sitter grammars are vendored under `crates/harbor-parsers/vendor/`. Current coverage: Rust, Go, TypeScript, Python, C, C++, Ruby. Adding a language is a half-day task: drop the grammar, write a `Language` impl with the node kinds that matter for symbol extraction, wire it into the dispatcher in `parsers/lib.rs`, and add a fixture repo under `tests/fixtures/<language>/`.

## Build & Test Commands

```bash
cargo build --release              # release binary lands in target/release/harbor
cargo test --workspace             # runs unit + integration across all crates
cargo xtask bench                  # criterion suite with baseline comparison
cargo xtask coverage               # tarpaulin coverage, html report in target/coverage/
cargo clippy --workspace -- -D warnings
cargo fmt --all --check
```

CI runs the full suite on PR. Benchmarks only run on the release branch since criterion is noisy on shared runners.

## Code Standards

### Rust Conventions

- Prefer `&str` over `String` in function signatures; own at the boundary
- No `unwrap()` or `expect()` outside tests and `main` — use `?` or `anyhow::Context`
- Errors crossing crate boundaries are concrete enum types that impl `std::error::Error`, never `anyhow::Error`
- `unsafe` blocks require a `// SAFETY:` comment above them stating the invariants; every `unsafe` gets a review from a second eyes
- Async code uses tokio; never mix tokio and async-std
- Public types derive `Debug`; avoid deriving `Clone` unless a caller demonstrably needs it

### Indexing Discipline

- The tantivy schema is append-only. Schema changes require a shard migration, not an in-place edit
- Document IDs are stable: `<repo_hash>:<file_hash>:<symbol_range>` — any change to the scheme breaks reproducibility
- Full reindex must complete within 2x the prior baseline; regressions fail CI via `cargo xtask bench-diff`

### TUI Discipline

- Every keybinding is declared in `harbor-tui/src/keymap.rs`; no inline `KeyEvent` matching scattered through widgets
- Widgets render deterministically from state — no I/O inside `Widget::render`
- Side effects are modeled as commands dispatched to a tokio channel, handled by the event loop

## Permissions

This is a read-heavy project. The opencode config (`opencode.json`) grants:

- `edit: allow` — Rust sources and test fixtures
- `bash: ask` — any shell command still prompts, since cargo builds can pull network deps
- `webfetch: deny` — the project pins docs.rs links; online lookups are a trap for version drift

See `opencode.json` for the full permission matrix and the per-agent overrides.

## MCP Integrations

- `harbor-index` — a local MCP server exposing the live tantivy index for query inspection during development
- `linear` — fetches the current sprint's tickets for context when planning work

## Agents

Two custom agents live under `agents/`:

- `review` — subagent for targeted code review; focused on unsafe blocks, error handling, and tantivy schema drift
- `plan` — primary agent in plan mode; produces a numbered implementation plan with verification steps before any code runs

The built-in opencode Build and Plan agents handle day-to-day coding; custom agents are invoked by name when a specialist lens is useful.

## Release Process

Releases are cut from `main` via `cargo xtask release <version>`. The xtask bumps workspace versions, regenerates the CHANGELOG from conventional-commit subjects, tags the commit, and pushes. GitHub Actions builds binaries for linux-gnu, linux-musl, macos-arm64, and windows-msvc, then attaches them to the release. The musl build is the one shipped in the Docker image.

## Instructions Loading

The opencode `instructions` field references `memory/*.md` via glob so lazy-loaded context — user preferences, project state, testing feedback — is always in scope during a session. Agent-specific prompts are loaded only when that agent is selected.
