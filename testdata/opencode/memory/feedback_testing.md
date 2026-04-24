---
name: Testing conventions for harbor
description: User guidance on integration tests, proptest usage, and what not to mock
type: feedback
---

# Testing Feedback

Integration tests must build a real tantivy index against a fixture repo — never mock the tantivy `IndexWriter`.

**Why:** A mocked `IndexWriter` hid a schema registration order bug for three weeks. The mock accepted field writes the real writer rejects at commit time. The bug only surfaced during a release build where tantivy's commit path runs.

**How to apply:** Every test that touches indexing spins up a `tempfile::tempdir()`, builds the real index, and asserts against query results. Integration tests live under `tests/` at the workspace root, not inside `crates/harbor-core/tests/`, so they can depend on `harbor-cli` for end-to-end coverage.

---

Use `proptest` for any function with non-trivial input space — token normalizers, path globbing, query parsers. Avoid enumerating examples by hand when a property holds.

**Why:** A hand-written test suite for the glob matcher missed a case where `**/` at the start of a pattern behaved differently from `**/` in the middle. `proptest` found it in under 100 generated inputs. Since then, any function accepting a `&str` pattern or an arbitrary-shape AST gets a property test alongside the example-based tests.

**How to apply:** Property tests live in `#[cfg(test)] mod proptests` blocks inside the module they cover. Use `proptest::prelude::*` and configure with `ProptestConfig { cases: 256, ..Default::default() }` unless the function is slow enough to warrant fewer iterations. Shrinking is on by default — leave it on.

---

Do not write TUI snapshot tests against the rendered buffer.

**Why:** Snapshot tests of the ratatui buffer broke on every Unicode width change in the upstream library. The signal-to-noise ratio was terrible — we spent more time rerecording snapshots than catching real regressions. The current rule is: test the state machine that feeds the widgets, not the rendered output.

**How to apply:** Widgets are `Widget` impls that render deterministically from a state struct. Tests assert against the state struct transitions, not against the buffer. End-to-end TUI behavior is covered by a tiny set of expect-style tests that drive the event loop with synthetic `KeyEvent`s and assert on the resulting state.
