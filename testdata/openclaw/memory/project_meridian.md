---
agent: atlas
type: project-spec
project: meridian
owner: platform-team
---

# Meridian

Meridian is the company's public API platform. It exposes business capabilities — catalog, pricing, inventory, orders, fulfillment — to external partners under a unified authentication, rate-limiting, and billing surface. Meridian sits in front of roughly 40 internal services; partners never call those services directly. Everything goes through the gateway.

## Why Meridian Exists

Before Meridian, every partnership was a bespoke integration: a new subdomain, a new auth scheme, a new rate-limit policy, and a new on-call rotation per partner. By Q3 2024 we had 17 partner-specific auth layers and the on-call burden had become unsustainable. Meridian consolidates the partner-facing surface so internal services stay focused on business logic, not on auth, rate limiting, billing, or partner SLA tracking.

## System Components

### API Gateway

The entrypoint. Implemented in Go using a fork of the `go-chi` router with a custom middleware stack. The gateway handles:

- TLS termination via the edge load balancer
- Partner authentication (API key, OAuth2 client credentials, mutual TLS)
- Scope-based authorization against the declared endpoint scopes
- Request rate limiting per partner, per endpoint, and globally
- Request routing to the appropriate internal service
- Response shaping to enforce the public API envelope
- Billing-event emission on metered endpoints

The gateway is stateless, horizontally scalable, and currently runs 24 pods across two availability zones. Target p99 latency added by the gateway itself is under 15ms.

### Auth Service

Issues and validates partner credentials. Partners can be issued API keys (simple, used by 80% of integrators) or OAuth2 client credentials (preferred for partners with multiple users or rotating secrets). Tokens carry scopes and an expiry; the gateway caches validated tokens for 60 seconds to reduce auth-service load.

Auth is the hottest path in the platform. It runs on dedicated pods with a tuned JVM (yes, this one is Java — the rest of Meridian is Go) because the original JWT library had a 3× throughput advantage in Java that we have not yet rewritten.

### Rate Limiter

Token-bucket algorithm backed by Redis sorted sets. Each partner has a budget per (endpoint, window) pair, configurable via the ops UI. Default budget is 100 req/min per endpoint per partner; negotiated partners can be raised up to 10,000 req/min with platform team approval. The limiter emits `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset` headers on every response and `Retry-After` on 429s.

WebSocket policy is still under active decision — whether a persistent connection should count against the token bucket or get a separate concurrency cap. Tracked as DATA-2205.

### Billing Collector

Consumes the gateway's billing-event stream (Kafka topic `meridian.billing.v1`) and writes aggregated usage rows to the billing database every 5 minutes. Invoices are generated monthly by a separate service that reads from this database. The collector is the only component that depends on Kafka — everything else uses direct HTTP or the internal service mesh.

### Partner Portal

The partner-facing web UI at `partners.meridian.example.com`. Used by partner engineers to view API keys, rate-limit status, usage dashboards, invoices, and documentation. Built in Next.js and served separately from the API. Single sign-on via the partner SSO provider (Auth0).

### Observability Stack

- **Metrics:** Prometheus scrapes every gateway pod; Grafana dashboards at `grafana.internal/d/meridian`
- **Tracing:** OpenTelemetry with OTLP to Tempo; 10% head-sampling with 100% tail-sampling for errors
- **Logs:** structured JSON via `zerolog`, shipped to Loki; PII never leaves the service boundary
- **Errors:** Sentry catches panics and unhandled errors; severity levels tuned to avoid alert fatigue

## API Design

### Versioning

URL-path versioning: `/v1/...`, `/v2/...`. Major versions run in parallel for at least 12 months before the older is sunset. Minor changes (additive fields, new optional parameters) go into the current major without a version bump. Every response carries an `X-API-Version` header with the effective version served.

### Pagination

Cursor-based pagination everywhere. Request: `?page_size=50&page_token=<opaque>`. Response: `{"items": [...], "next_page_token": "<opaque-or-empty>"}`. Offset pagination is forbidden because it's unstable under concurrent writes.

Default page size is 50, max is 500. Requests over the max are truncated silently to preserve backward compatibility — no errors for size overages.

### Error Format

RFC 9457 problem-details for 4xx and 5xx:

```json
{
  "type": "https://errors.meridian.example.com/rate-limited",
  "title": "Rate limit exceeded",
  "status": 429,
  "detail": "Exceeded budget of 100 req/min on endpoint /v1/orders",
  "instance": "/req/a1b2c3d4",
  "retry_after": 42
}
```

Every error carries a unique `instance` URI that partners can use when opening a support ticket. Internal error IDs are *not* exposed — the `type` URI points to public documentation.

### Idempotency

Mutating endpoints (POST, PUT, PATCH) honor an `Idempotency-Key` header. The gateway caches the response keyed by `(partner_id, path, idempotency_key)` for 24 hours. Repeated requests with the same key return the cached response verbatim, even if the original response was a 4xx — idempotency is about request repetition, not about retrying failures.

## Authentication & Authorization

Three auth modes supported:

1. **API Key** — simple bearer token in the `Authorization` header; suitable for server-to-server integrations where the partner controls the environment
2. **OAuth2 Client Credentials** — partners with multiple backend services or rotating secrets get client_id/client_secret and exchange for short-lived access tokens
3. **Mutual TLS** — reserved for high-value partners with hard compliance requirements; partner presents a client cert issued by our internal CA, we validate against the allowlist

Scopes are declared at the endpoint level (`orders:read`, `orders:write`, `inventory:read`, etc.) and granted to partners via the partner portal. The platform team approves scope grants; automated onboarding covers the common low-sensitivity scopes without a human in the loop.

## Data Model

### Partner

`id`, `name`, `tier` (basic, partner, strategic), `onboarded_at`, `tech_contact_email`, `billing_contact_email`, `active`. One-to-many with `api_keys`, `oauth_clients`, and `rate_limit_overrides`.

### Usage

Rolled-up usage per `(partner_id, endpoint, hour)`. The source of truth for billing. Retained indefinitely because billing disputes can reach back arbitrarily.

### Webhook Subscription

Partners can subscribe to event types (`order.created`, `inventory.depleted`, etc.). The webhook service delivers events with HMAC-SHA256 signatures in the `X-Meridian-Signature` header. Signing key rotated every 90 days; the rotation window overlaps by 7 days so partners have time to update.

### Incident

Platform incidents that affect partner traffic. When an incident is declared, affected partners get a webhook (`platform.incident.opened`) and the incident appears on the public status page at `status.meridian.example.com`.

## Rate Limiting

### Token Bucket

Each (partner, endpoint) pair has a bucket with a capacity (burst) and a refill rate (sustained). Default: capacity 100, refill 100/min. The bucket is implemented as a Redis sorted set of request timestamps within the last 60 seconds.

### Headers

Every response carries the current bucket state:

- `X-RateLimit-Limit` — the sustained rate (refill per minute)
- `X-RateLimit-Remaining` — tokens left in the bucket right now
- `X-RateLimit-Reset` — unix timestamp when the bucket will be full again
- `Retry-After` — only on 429, seconds to wait

### Budgets

Internal service-to-service calls bypass the limiter via the `X-Internal-Token` header. The token rotates daily and is distributed via the service mesh's secret injection.

## Security

- All partner traffic is TLS 1.3 with PFS cipher suites; TLS 1.2 deprecated 2025-12-31
- Secrets (API keys, OAuth secrets, HMAC signing keys) rotate on a schedule enforced by Vault; the rotation job runs nightly and alerts if any secret exceeds its TTL
- Webhook payloads include the HMAC signature and a monotonic `event_id` so partners can detect replay attacks
- CSP, HSTS, and X-Frame-Options are set by the gateway on every response (yes, including API responses — defense in depth)
- Input validation is layered: gateway checks envelope, each service validates its own payload; never trust the adjacent service

## Partner Integration

### Onboarding

1. Partner signs the platform agreement; legal creates a `Partner` row
2. Partner engineer logs into the portal with the tech contact email, provisions an API key, sees documentation scoped to their tier
3. Sandbox environment issues its own API keys — sandbox traffic never touches production data
4. Graduation to production requires partner to pass the compliance checklist (`partners.meridian.example.com/checklist`) and request promotion via the portal; platform team reviews and flips the flag

### Staging vs Production

Sandbox and production are physically separated: different database clusters, different Redis, different Kafka. Credentials issued for one do not work on the other. Data flows from production → sandbox nightly via a sanitization pipeline that strips PII, redacts pricing, and randomizes identifiers.

### SLAs

Per-tier SLAs:

- **Basic:** best-effort; no commitment
- **Partner:** 99.9% availability, p99 latency < 500ms for read, < 800ms for write; business-hours support
- **Strategic:** 99.95% availability, dedicated capacity pool, 24/7 support, direct escalation to platform team lead

SLA violations trigger automated partner notifications and a root-cause writeup delivered within 5 business days.

## Incident Response

### Severities

- **SEV1** — customer-impacting outage or data loss; pages on-call immediately, incident commander engaged, public status page updated
- **SEV2** — degraded service (elevated errors, slowness) affecting multiple partners; pages on-call, status page updated
- **SEV3** — single-partner issue or internal-only degradation; Slack alert, business-hours response
- **SEV4** — precautionary; tracked in the incident log but no paging

### Runbooks

Runbooks live in `runbooks/` in the platform repo, one per common failure mode. The gateway has dedicated runbooks for auth-service outage, rate-limiter Redis failure, and internal-service cascading failure. Every on-call engineer must read the top-5 runbooks on their first rotation.

### Post-mortems

Every SEV1 and SEV2 gets a blameless post-mortem within 5 business days. Published internally, key findings summarized publicly when partners were affected.

## Release Process

- **Gateway:** weekly release, Tuesday 10:00 UTC; canary to 5% → 50% → 100% over 4 hours; automatic rollback on error rate > 0.5% above baseline
- **Auth service:** biweekly release, same day as gateway; stricter canary (1% → 10% → 100%) because it's the hottest path
- **Rate limiter, billing collector:** on-demand; typically 1–2 per week per service
- **Partner portal:** continuous deploy on merge to main; feature flags gate user-visible changes

## Open Questions

- **WebSocket rate-limit policy** — per-connection bucket vs per-message bucket? Jordan to decide by 2026-04-22; tracked as DATA-2205
- **Tiered pricing for large partners** — current flat per-request pricing becomes punitive above ~5M req/day; finance is modeling a tiered alternative
- **GraphQL layer** — partner feedback has asked for a GraphQL entry point; platform team is skeptical (operational cost, query-depth attacks) but the product team wants a spike in Q3
- **Sovereign data regions** — at least two strategic partners require EU-only data residency; requires gateway-level routing by partner region, plus region-specific database clusters

## Glossary

- **Partner** — a company or individual with a signed platform agreement; has one or more API keys or OAuth clients
- **Tenant** — a namespace within a partner; large partners have multiple tenants for their own internal separation
- **Scope** — a named permission granted to a partner (e.g., `orders:read`) that maps to a set of endpoints
- **Bucket** — a rate-limit token-bucket instance, keyed by `(partner, endpoint)`
- **Event** — a webhook-delivered notification; event types are versioned (`order.created.v1`)
- **Envelope** — the standard response shape: `{"data": ..., "meta": {...}, "errors": [...]}`
