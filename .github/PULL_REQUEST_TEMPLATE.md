## Summary

Closes #<!-- required: replace with the GitHub issue number this PR addresses (e.g. `Closes #42`). Use `Refs #` if the PR should not auto-close on merge. -->

<!-- 1–2 sentences on the "why". -->

## Self-review

### Touched

**Pipeline**

- [ ] `pkg/parser/` — fuzz target covers any new shape
- [ ] `pkg/transformer/` or `pkg/diff/` — ID stability / hash inputs unchanged
- [ ] `pkg/emitter/` — exactly one snapshot per write
- [ ] `pkg/store/` + `migrations/` — FTS5 triggers in sync; `add-store-query` followed
- [ ] `pkg/query/` or `pkg/compiler/` — token budget honored

**MCP surface**

- [ ] `pkg/mcp/` — `skills/remind` (read) or `skills/memoize` (write) updated
- [ ] `pkg/temperature/` — both public skills reflect new values

**Edges**

- [ ] `cmd/remindb/` — README CLI section updated
- [ ] `internal/ignore/`, `internal/tempfile/`, `internal/bench/` — relevant fuzz / fixture / scenario added
- [ ] `plugins/<agent>/` — manifest version bumped if shipping

### Process

- [ ] Tested manually via CLI or local MCP plugin install
- [ ] `make test-all` + `make fuzz` green
