---
agent: atlas
type: project
created: 2026-04-16
---

# API Rate Limiting

Implemented token bucket rate limiter in `pkg/ratelimit/` using `golang.org/x/time/rate`. Deployed to staging on 2026-04-16.

## Configuration

- Authenticated requests: 100 per minute per IP
- Anonymous requests: 20 per minute per IP
- Middleware returns HTTP 429 with `Retry-After` header on exhaustion
- Integration test covers burst behavior and header correctness

## Open Items

- Pending: rate limit policy for WebSocket connections — need Jordan's decision on whether persistent connections count against the bucket
- Pending: Sentry alert threshold review — current threshold generates too many false positives during deploy windows
- TODO left in middleware linking to design doc at `docs.meridian.internal/api/rate-limiting`

**Why:** Public API had no rate limiting, leaving it vulnerable to abuse and accidental DoS from misbehaving clients. The 100/20 split was Jordan's call based on current traffic patterns.

**How to apply:** All new public endpoints must be wrapped with the rate limiter middleware. Internal service-to-service calls bypass it via the `X-Internal-Token` header.
