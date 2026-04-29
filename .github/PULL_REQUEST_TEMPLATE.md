## Summary

<!-- 1–2 sentences on the "why". -->

## Self-review

### Verified

- [ ] Tested manually via CLI or local MCP plugin install
- [ ] Ran the test suite with fuzzing (`make test-all`, `make fuzz`)

### Touched

- [ ] **MCP tools or public skills** — code and skill stay in sync (`skills/remind/`, `skills/memoize/`)
- [ ] **Temperature config** — both public skills reflect new values
- [ ] **Parser** — fuzz target covers the change
- [ ] **Schema or migrations** — FTS5 triggers in sync
- [ ] **Write path** — each write tool call still produces exactly one snapshot
- [ ] **Client output format** — unchanged, or breaking change noted in summary
- [ ] **CLI** — user-facing changes documented in summary
