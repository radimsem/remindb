---
name: External references for harbor development
description: Where to find tickets, dashboards, upstream docs, and incident history
type: reference
---

# References

## Issue Tracking

- Linear project **HARBOR** (slug `harbor`) tracks all feature work and bugs; URL: https://linear.app/harbor
- Sprint-in-progress filter: https://linear.app/harbor/view/current-sprint
- Incident tickets carry the `incident` label and are pinned to the top of the backlog until resolved
- Triage inbox: any issue without a cycle is reviewed at the Monday standup by the tech lead

## Upstream Documentation

- tantivy book: https://fulmicoton.gitbooks.io/tantivy/content/
- ratatui docs: https://docs.rs/ratatui/latest/ratatui/
- tree-sitter grammar development guide: https://tree-sitter.github.io/tree-sitter/creating-parsers
- crossterm event model: https://docs.rs/crossterm/latest/crossterm/event/

## Dashboards

- Index health (Grafana): https://grafana.harbor.internal/d/index-health — tracks reindex latency, shard count, and segment merge rate
- Query latency (Grafana): https://grafana.harbor.internal/d/query-latency — p50, p95, p99 by query type, with a separate panel for MCP tool calls
- CI throughput (Grafana): https://grafana.harbor.internal/d/ci-throughput — runner wait times, test durations per crate, flaky test surface
- Crate dependency graph (Datadog): https://app.datadoghq.com/sci/deps/harbor — visualizes reverse deps for impact analysis

## Benchmarks

- Criterion reports land at https://bench.harbor.internal after every main-branch push
- Baselines live in `target/criterion/` on the release runner; use `cargo xtask bench-diff <sha>` to compare locally
- The reference monorepo for indexing benchmarks is a snapshot of the chromium source tree, stored in S3 at `s3://harbor-bench/chromium-2026q1/`

## Slack Channels

- `#harbor-core` — architectural discussion, RFCs, design reviews
- `#harbor-oncall` — active incidents; stays quiet except during paging events
- `#harbor-releases` — release announcements and rollback threads; read-only outside release windows

## Runbooks

- Oncall runbook: https://docs.harbor.internal/runbooks/oncall
- Release runbook: https://docs.harbor.internal/runbooks/release
- Schema migration runbook: https://docs.harbor.internal/runbooks/tantivy-shard-migration
