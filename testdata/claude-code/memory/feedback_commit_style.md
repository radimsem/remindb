---
name: Commit style feedback
description: Commit message preferences learned from past PR review
type: feedback
---

Write commit subjects in lowercase imperative mood with a conventional-commit type prefix. Do not exceed 72 characters. Body only when the *why* is not obvious from the diff.

**Why:** The team's squash-merge workflow uses the commit subject verbatim as the PR title. Long, mixed-case, or non-imperative subjects produce a messy PR list on the release-notes page which the EM reviews every Friday.

**How to apply:** Prefer `feat(cart): persist line items across devices` over `Added cart persistence across devices and fixed a related bug`. Split the two changes if both exist.

---

Do not reference PR numbers, issue IDs, or "as requested by X" in commit messages.

**Why:** Those references point at ephemeral things — a ticket closed, a person changed role, a PR got squashed into a different one. Six months later the reference is noise, not context.

**How to apply:** The *reason* belongs in the commit body ("fixes the race between checkout and inventory sync"), not the ticket ID. PRs and tickets link to commits, not the other way around.

---

One logical change per commit — even if it means more commits. A commit that mixes a bug fix with a refactor is impossible to revert cleanly.

**Why:** A revert during an incident is already stressful; discovering the revert pulls along an unrelated refactor makes it worse. Confirmed after the Q1 cart-persistence incident where the hotfix revert undid a useful type-narrowing cleanup.

**How to apply:** If you catch yourself writing "and" in the subject, stop and split. Refactors go in their own commit, before the feature that depends on them.
