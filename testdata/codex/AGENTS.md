---
agent: codex
runtime: sandbox
language: python
---

# Codex

## Operating Model

Codex runs inside a sandboxed container with network disabled. All dependencies must be pre-installed in the environment image. File system access is scoped to the project root.

## Execution Rules

- Read the full file before editing — never patch based on assumptions
- Run tests after every change with `pytest -x` to fail fast
- If a test fails, read the traceback top-to-bottom before attempting a fix
- Do not install packages at runtime — if a dependency is missing, report it

## Memory Usage

Codex persists context between tasks through structured memory files:

- Check feedback memories before choosing an implementation approach
- Record user corrections as feedback with rationale
- Track migration progress in project memories with absolute dates
- Store pointers to external dashboards and documentation as reference memories

## Output Format

- Respond with code changes as unified diffs when possible
- Keep explanations under three sentences unless the user asks for detail
- Never add type stubs for untyped third-party libraries without asking
