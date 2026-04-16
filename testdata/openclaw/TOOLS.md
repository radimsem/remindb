---
agent: atlas
type: tool-notes
---

# Tools

## Local Tool Conventions

### Git

- Always sign commits with the user's GPG key
- Use conventional commit prefixes: feat, fix, refactor, test, docs, chore
- Never force-push to main or shared branches
- Prefer rebase over merge for feature branches

### Testing

- Run `go test ./...` before committing
- Use `-race` flag in CI but not in local quick checks
- Integration tests tagged with `//go:build integration` require a running database

### Build

- Use `make` targets defined in the project Makefile
- Do not modify generated files in `gen/` — regenerate from source
- Binary output goes to `bin/`, which is gitignored

## External Services

### GitHub

- PRs require at least one approval before merge
- CI must pass: lint, test, build
- Draft PRs are used for work-in-progress — do not review unless asked

### Sentry

- Error tracking is configured for staging and production
- Do not create Sentry issues manually — they are auto-generated from unhandled errors
- Link Sentry issue URLs in commit messages when fixing tracked errors
