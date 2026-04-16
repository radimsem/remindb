---
agent: atlas
type: tool-notes
---

# Tools

## Local Tool Conventions

### Git

Jordan's repository requires signed commits for audit compliance. Unsigned commits are rejected by the pre-receive hook on the remote, so failing to sign locally means a wasted push-and-fix cycle. The conventional commit format is enforced by a commit-msg hook that validates the prefix against an allowlist. Rebase is preferred over merge to keep the history linear and make bisecting practical.

- Always sign commits with the user's GPG key — verify the agent is running before the first commit of a session
- Use conventional commit prefixes: feat, fix, refactor, test, docs, chore
- Never force-push to main or shared branches — this destroys signed commit provenance
- Prefer rebase over merge for feature branches to maintain linear history

### Testing

The test suite is the primary safety net for refactoring. Running tests before every commit catches regressions immediately rather than deferring them to CI, where the feedback loop is 3-5 minutes. The race detector is excluded from local runs because it adds 2-10x overhead and local iteration speed matters more than exhaustive race coverage. CI runs always include the race detector because correctness under concurrency is non-negotiable for production.

- Run `go test ./...` before committing to catch regressions locally
- Use `-race` flag in CI but not in local quick checks to keep iteration fast
- Integration tests tagged with `//go:build integration` require a running PostgreSQL database on localhost:5432

### Build

- Use `make` targets defined in the project Makefile
- Do not modify generated files in `gen/` — regenerate from source
- Binary output goes to `bin/`, which is gitignored

## External Services

### GitHub

The repository follows a trunk-based development model with short-lived feature branches. PRs are the unit of code review and must pass all CI checks before merge. The branch protection rules on main enforce this — direct pushes are blocked. Draft PRs signal work-in-progress and should not be reviewed because reviewing incomplete code produces feedback that becomes stale once the implementation is finished.

- PRs require at least one approval before merge — Atlas can create PRs but cannot approve its own
- CI must pass: lint (golangci-lint), test (go test -race), build (go build) for all target platforms
- Draft PRs are used for work-in-progress — do not review unless the author explicitly requests feedback

### Sentry

- Error tracking is configured for staging and production
- Do not create Sentry issues manually — they are auto-generated from unhandled errors
- Link Sentry issue URLs in commit messages when fixing tracked errors
