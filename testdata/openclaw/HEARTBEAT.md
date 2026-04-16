---
agent: atlas
type: heartbeat
interval: 6h
---

# Heartbeat

Lightweight checklist for automated background runs every 6 hours.

## Checks

The heartbeat checks are ordered by cost and impact. Pulling from main is cheap and catches integration issues early. Running the test suite is more expensive but catches regressions introduced by other contributors between sessions. Sentry checks surface production issues that may require immediate attention. Memory staleness review is lowest priority because stale memories cause suboptimal suggestions, not failures.

1. Pull latest changes from `main` and check for merge conflicts with local branches — if conflicts exist, log them but do not attempt automatic resolution
2. Run `go test ./...` and compare against the last heartbeat's results to identify newly failing tests
3. Check Sentry for unacknowledged errors in the last 6 hours, filtering out known flaky errors listed in the project memory
4. Scan `memory/` for entries with access_count of zero and age greater than 14 days, flagging them for review

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

The output is written as a project memory so that future sessions can see the health trend without re-running the checks. The summary should be factual and quantitative — do not editorialize about whether the results are "good" or "concerning." The next session will interpret the numbers in its own context.

Write a one-paragraph summary to the daily memory log. Include:

- Number of tests run, number passing, number failing, and names of any newly failing tests
- Number of new Sentry errors found with their severity levels and affected services
- Number of stale memories flagged and the oldest entry's age in days
- Timestamp of the next scheduled heartbeat and whether any checks were skipped due to errors
