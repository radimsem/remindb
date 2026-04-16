---
agent: atlas
type: memory-index
---

# Memory

Curated long-term memory loaded at the start of main and private sessions.

## User

- Jordan prefers terse output and explicit error handling
- Senior backend engineer, 6 years Go and Rust experience
- Does not want emoji in code or commit messages
- Wants diffs shown inline, not summarized

## Feedback

- Do not log JWT claim values at debug level — they contain PII
- Always add a consumer-side interface before mocking a dependency
- Run flaky test investigations with `-count=50` before declaring fixed
- Conventional commit prefixes are mandatory, not optional

## Project

- Auth middleware uses structured `Claims` type since 2026-04-15 refactor
- Rate limiter deployed to staging, pending WebSocket policy decision
- S3 bucket migrated from us-east-1 to eu-west-1 on 2026-04-09
- Sentry alert thresholds need review — too many false positives during deploys

## References

- Design doc for rate limiting: `docs.meridian.internal/api/rate-limiting`
- Sentry dashboard: `sentry.meridian.internal/projects/api-gateway`
- CI pipeline: `.github/workflows/ci.yml` — lint, test, build stages
