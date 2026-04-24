---
name: User coding preferences for harbor sessions
description: Senior Rust engineer, prefers unified diffs, terse responses, no emojis
type: user
---

# User Profile

User is a senior systems engineer with 12 years of Rust and C++ experience. Currently the maintainer of the harbor project and the primary reviewer for anything touching `harbor-core`.

## Response Style

- Terse. One-sentence answers when the question admits one; bullets over paragraphs when structure helps
- No emojis. Anywhere. Not in code, not in comments, not in responses
- No preamble. No "I'll help you with that" — go straight to the answer
- Unified diffs for all code changes: `diff -u` format with file paths, not isolated blocks
- When proposing a refactor, show the before/after side by side; do not ask the user to imagine the result

## Technical Preferences

- `?` and `anyhow::Context` over match-and-rewrap chains for error handling
- Concrete enum error types crossing crate boundaries; `anyhow::Error` only inside `main` and binary entry points
- Prefer `&str` in function signatures; own the `String` at the boundary only if necessary
- `Vec::with_capacity` when the size is known; do not rely on the default doubling strategy in hot loops
- No `derive(Clone)` on types unless a caller demonstrably needs it — derives creep and create accidental performance cliffs
- Explicit lifetimes on any signature that Rust's elision rules handle ambiguously; readability wins over terseness here

## Workflow Preferences

- Plan before building: invoke the `plan` agent for anything non-trivial; if the user skips planning, do not insist
- Surface risks before acting, not after; if there are genuinely no risks, say so — do not invent one
- Never commit without the user asking; never push without asking twice
- When a build fails, read the full cargo output before proposing a fix — the real error is rarely the first line

## Tooling

- Editor: Helix with the harbor config preset; LSP via rust-analyzer nightly
- Terminal: Alacritty with a 2-pane layout (code + harbor TUI)
- Prefer `rg` over `grep` and `fd` over `find` for ad-hoc local searches, even though harbor is the primary search tool for the harbor codebase itself
