---
name: Tantivy migration from legacy SQLite FTS
description: Progress on replacing the sqlite-fts5 backend with tantivy for harbor's search index
type: project
---

# Tantivy Migration Status

## Status (2026-04-20)

Migrating the legacy sqlite-fts5 backend to tantivy for the full-text index. 6 of 9 subtasks complete; remaining three are gated on external dependencies or review.

## Completed

- Schema design, reviewed and merged (LIN-2104)
- Tantivy indexer for Rust and Go grammars (LIN-2105)
- Query planner adapter — translates the existing `SearchQuery` AST to tantivy's `Query` trait (LIN-2106)
- Parallel indexing pipeline with rayon, 4x speedup on the reference monorepo (LIN-2108)
- Shard migration tool `cargo xtask migrate-shards` that upgrades on-disk schemas without full reindex (LIN-2110)
- CLI parity: `harbor search` produces identical result ordering against tantivy and sqlite-fts5 on the regression corpus (LIN-2112)

## Remaining

- TUI result ranking — tantivy's BM25 scores have a different magnitude than fts5 bm25; the TUI highlight thresholds need retuning (LIN-2107, blocked on UX review scheduled 2026-04-28)
- Symbol-aware boosting — tantivy's custom scorer trait needs a per-symbol-kind weight; blocked on the decision about whether to expose the weights as a user-facing config or hardcode them (LIN-2109, design spike open)
- sqlite-fts5 removal — cannot drop until the on-call runbook is updated and the rollback plan is signed off by the release manager (LIN-2114, blocked on runbook review)

**Why this order:** Index correctness and query compatibility had to ship before any ranking work, since ranking tweaks are meaningless if the candidate set is wrong. Removing sqlite-fts5 last gives us a one-command rollback for two release cycles.

**How to apply:** Do not start LIN-2109 without confirming the design decision on weight exposure. Do not merge a PR that removes any sqlite-fts5 code paths until LIN-2114 lands. The tantivy and sqlite-fts5 indexes ship side by side in 0.5.x; the `--backend` flag selects between them for debugging.

## Next Milestones

- 2026-05-05: TUI ranking retune lands, feature-flagged behind `harbor.experimental.tantivy_ranking`
- 2026-05-19: sqlite-fts5 code paths deprecated, compile-time warning emitted from the legacy backend module
- 2026-06-02: 0.6.0 release — tantivy default, sqlite-fts5 gated behind `--features legacy-fts5`
