---
agent: codex
runtime: sandbox
language: python
---

# Codex

## Operating Model

Codex runs inside a sandboxed container with network disabled. All dependencies must be pre-installed in the environment image. File system access is scoped to the project root.

## Execution Rules

The sandbox constraint fundamentally shapes how Codex operates. Because the container has no network access and a fixed dependency set, Codex cannot pip-install missing packages or fetch remote resources at runtime. This means every change must work with the pre-installed versions of pandas, polars, sqlalchemy, pydantic, and the other packages in the environment image. If a task requires a package that is not installed, Codex must report the gap rather than silently failing or attempting a workaround.

- Read the full file before editing — never patch based on assumptions about surrounding code
- Run tests after every change with `pytest -x` to fail fast on the first broken test
- If a test fails, read the traceback top-to-bottom before attempting a fix — the root cause is usually in the innermost frame
- Do not install packages at runtime — if a dependency is missing, report it with the exact import that failed

## Memory Usage

Memory is Codex's mechanism for learning across sessions. Without memory, every task starts from zero context and Codex repeats mistakes that the user already corrected. The memory system stores four types of entries: user preferences that shape how Codex communicates, feedback that records what the user corrected and why, project state that tracks ongoing work like the ETL migration, and references that point to external systems like the Snowflake dashboard or the Airflow DAG monitor.

Codex persists context between tasks through structured memory files:

- Check feedback memories before choosing an implementation approach — a past correction may rule out an otherwise reasonable strategy
- Record user corrections as feedback with rationale, including what was wrong and what the user preferred instead
- Track migration progress in project memories with absolute dates so that status is unambiguous across sessions
- Store pointers to external dashboards and documentation as reference memories for quick lookup

## Output Format

- Respond with code changes as unified diffs when possible
- Keep explanations under three sentences unless the user asks for detail
- Never add type stubs for untyped third-party libraries without asking
