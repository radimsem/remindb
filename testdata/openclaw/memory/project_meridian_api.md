---
agent: atlas
type: project
project: meridian
created: 2026-04-12
updated: 2026-04-16
---

# Meridian Public API

## Current focus

Rolling out a token-bucket rate limiter to the public API ahead of the Q2 partner launch. Launch date target: 2026-05-15. Partners are already integrated in staging and expect the rate-limit headers to be in place before they cut over.

## What shipped

- Core limiter implementation in `pkg/ratelimit/limiter.go` (DATA-2203)
- Middleware integration in `pkg/middleware/ratelimit.go` (DATA-2204)
- Per-route limits configurable via `config/ratelimits.yaml`
- Response headers: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, `Retry-After` on 429

## What's pending

- WebSocket rate limit policy — whether persistent connections count against the token bucket (DATA-2205, blocked on Jordan's decision)
- Metrics: rate-limit hit counter per client-id → Prometheus
- Runbook entry for the SRE team covering how to raise a client's limit for a known partner
- Dashboard panels in Grafana (waiting on metrics)

## Why it matters

**Why:** A partner-load incident on 2026-03-20 saturated the shared PostgreSQL connection pool for 11 minutes and affected unrelated traffic. A handful of partners without negotiated limits accounted for 94% of the request volume. Token buckets isolate partner misbehavior so one noisy partner cannot cascade.

**How to apply:** When editing the API, if a new endpoint is added, it must be listed in `config/ratelimits.yaml` with a conservative default. If the endpoint streams data, coordinate with Jordan on WebSocket policy before merging.

## Open decisions

- WebSocket: per-connection token bucket vs per-message bucket? Jordan to decide by 2026-04-22.
- Rate-limit response body for 429: plain text vs JSON-LD with machine-readable retry info? Leaning JSON-LD to match RFC 9457 problem details.
- Limit configuration live-reload vs deploy-only? Live reload would help incident response but adds operational surface. Currently: deploy-only.
