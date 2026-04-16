---
agent: atlas
type: heartbeat
interval: 6h
---

# Heartbeat

Lightweight checklist for automated background runs every 6 hours.

## Checks

1. Pull latest changes from `main` and check for merge conflicts with local branches
2. Run `go test ./...` and report any new failures since last heartbeat
3. Check Sentry for unacknowledged errors in the last 6 hours
4. Scan `memory/` for entries flagged as stale and list them for review

## Notifications

Only notify Jordan (via configured webhook) if:

- A previously passing test now fails
- An unacknowledged Sentry error has severity `critical`
- A stale memory entry is older than 14 days and was never reviewed

Do not notify for:

- Successful runs with no findings
- Warnings or info-level Sentry events
- Memory entries that have been accessed within the last 7 days

## Output

Write a one-paragraph summary to the daily memory log. Include:

- Number of tests run and their pass/fail status
- Number of new Sentry errors found
- Number of stale memories flagged
- Timestamp of the next scheduled heartbeat
