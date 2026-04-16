---
name: API Gateway Architecture
description: Design decisions and implementation notes for the gateway service
type: project
---

# API Gateway Service

The gateway handles all external traffic and routes requests to internal microservices.
It enforces authentication, rate limiting, and observability across the fleet.

## Authentication

### JWT Validation

All requests must include a valid JWT in the Authorization header.
The gateway validates tokens against the auth service's JWKS endpoint.
Token refresh happens transparently when the access token is within 5 minutes of expiry.

```go
func validateToken(token string) (*Claims, error) {
    parsed, err := jwt.Parse(token, keyFunc)
    if err != nil {
        return nil, fmt.Errorf("failed to parse: token: %w", err)
    }
    claims, ok := parsed.Claims.(*Claims)
    if !ok || !parsed.Valid {
        return nil, ErrInvalidClaims
    }
    return claims, nil
}
```

### API Key Authentication

Service-to-service calls use API keys rotated every 90 days.
Keys are hashed with SHA-256 before storage. Plain keys exist only in vault.

### OAuth2 Flows

External integrations use OAuth2 authorization code flow.
Redirect URIs are validated against a strict allowlist per client ID.

## Rate Limiting

### Token Bucket Algorithm

Rate limits are enforced per-client using a token bucket with Redis backing.
Each tier gets a different bucket configuration.

| Tier | Requests/min | Burst | Cooldown |
|------|-------------|-------|----------|
| Free | 60 | 10 | 60s |
| Pro | 600 | 100 | 30s |
| Enterprise | 6000 | 1000 | 10s |

### Distributed Rate Limiting

Redis cluster stores bucket state. Lua scripts ensure atomicity.
On Redis failure, the gateway falls back to local in-memory buckets with 2x the normal limit.

## Routing

### Path-Based Routing

Routes are matched by longest prefix first:

- `/api/v1/users` → user-service
- `/api/v1/orders` → order-service
- `/api/v1/payments` → payment-service
- `/api/v1/notifications` → notification-service
- `/api/v1/analytics` → analytics-service
- `/api/v1/inventory` → inventory-service
- `/api/v1/shipping` → shipping-service
- `/api/v1/search` → search-service

### Load Balancing

Round-robin with health checks every 10 seconds.
Unhealthy backends are removed from the pool after 3 consecutive failures.
Backends are re-added after 2 successful health checks.

### Circuit Breaker

Trips after 5 failures in 30 seconds. Half-open after 60 seconds.
State is per-backend, not per-route. Prometheus gauge tracks breaker state.

## Middleware Stack

### Request Logging

Every request logs method, path, status, latency, client IP, and request ID.
Sensitive headers (Authorization, Cookie) are redacted in logs.

### CORS Configuration

```yaml
allowed_origins:
  - https://app.example.com
  - https://admin.example.com
  - https://docs.example.com
allowed_methods: [GET, POST, PUT, DELETE, PATCH]
allowed_headers: [Authorization, Content-Type, X-Request-ID]
max_age: 86400
credentials: true
```

### Response Compression

Responses larger than 1KB are compressed with gzip at level 6.
Static assets use pre-compressed Brotli variants when available.
WebSocket connections bypass compression entirely.

### Request Validation

JSON request bodies are validated against OpenAPI schemas.
Invalid requests return 422 with structured error details:

```json
{
  "error": "validation_failed",
  "details": [
    {"field": "email", "message": "must be a valid email address"},
    {"field": "age", "message": "must be between 18 and 150"}
  ]
}
```

## Error Handling

### Retry Policy

Retries: 3 attempts with exponential backoff (100ms, 200ms, 400ms).
Retry only on 502, 503, 504. Never retry mutations (POST, PUT, DELETE) unless idempotency key is present.

### Graceful Degradation

When a backend is unavailable, the gateway returns cached responses for GET requests.
Cache TTL is 5 minutes for degraded responses. Stale responses include a `X-Degraded: true` header.

## Monitoring

### Health Check Endpoint

`GET /health` returns service status and dependency health.
Dependencies checked: Redis, auth service JWKS, at least one backend per route.

### Prometheus Metrics

- `gateway_requests_total` — counter by method, path, status
- `gateway_request_duration_seconds` — histogram by path
- `gateway_active_connections` — gauge
- `gateway_circuit_breaker_state` — gauge by backend
- `gateway_rate_limit_rejections_total` — counter by tier
- `gateway_upstream_errors_total` — counter by backend, status

## Deployment

### Environment Variables

- `GATEWAY_PORT` — listen port (default: 8080)
- `GATEWAY_TLS_CERT` — TLS certificate path
- `GATEWAY_TLS_KEY` — TLS key path
- `GATEWAY_UPSTREAM_TIMEOUT` — upstream timeout (default: 30s)
- `GATEWAY_MAX_BODY_SIZE` — max request body (default: 10MB)
- `GATEWAY_LOG_LEVEL` — log level (default: info)
- `GATEWAY_REDIS_URL` — Redis connection string
- `GATEWAY_JWKS_URL` — JWKS endpoint for JWT validation

### Kubernetes Resources

```yaml
resources:
  requests:
    cpu: 500m
    memory: 256Mi
  limits:
    cpu: 2000m
    memory: 1Gi
```

Pod disruption budget: minAvailable 2. HPA scales between 3 and 20 replicas based on p99 latency.
