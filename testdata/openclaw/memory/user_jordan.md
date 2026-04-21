---
agent: atlas
type: user
user: jordan
created: 2026-03-05
updated: 2026-04-16
---

# Jordan

## Role

Staff engineer on the Meridian platform team. Owns the public API, the auth middleware, and the observability stack. Reports to the platform group lead.

## Working style

- Direct and terse. Does not want filler or affirmations. If there's uncertainty, Jordan wants the uncertainty named, not hidden.
- Prefers plain text over bullet lists when the content fits in one or two sentences.
- Reviews code in hunks, not whole files. Diffs under 200 lines land same-day; anything larger sits in review queue for ~2 days.
- Commits with conventional-commits prefixes. Uses `feat`, `fix`, `refactor`, `test`, `docs`. Will push back on commits without a scope like `feat(ratelimit): ...`.

## Technical context

- Ten years of Go, two years of TypeScript (Meridian's frontend stack).
- Deeply familiar with PostgreSQL, less so with ClickHouse — surface analogies from PG when explaining CH query behavior.
- Owns the CI config personally. Any change to `.github/workflows/*.yml` needs to be flagged for review.
- Runs fish shell, neovim, uses git from the command line only (no GUI).

## Boundaries

- Do not log PII. Jordan flagged a JWT claim log on 2026-04-15 and made it a hard rule. See `memory/feedback_no_pii_logging.md`.
- Do not auto-delete stale memory. See `memory/feedback_memory_hygiene.md`.
- Do not schedule work or reminders without asking — Jordan rejects calendar-like automation from agents.
- No emojis unless Jordan uses them first in the current thread.
