---
agent: atlas
type: feedback
created: 2026-04-15
---

# Do Not Log Claim Values

Do not log JWT claim values at debug level. Claims may contain PII such as email addresses, user IDs tied to real identities, and role assignments that reveal organizational structure.

**Why:** Jordan flagged this during the auth middleware refactor on 2026-04-15. The structured `Claims` type was being logged with `%+v` in debug mode, which would have shipped PII to the centralized log aggregator.

**How to apply:** When logging authentication events, log only the action outcome (success/failure), the claim type checked, and a truncated request ID. Never log the claim value itself. Use `slog.Group("auth", "action", "validate", "result", "ok")` pattern.
