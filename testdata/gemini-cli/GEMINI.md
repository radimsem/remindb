---
project: infra-api
language: go
runtime: kubernetes
---

# Project Instructions

## Overview

Infrastructure management API written in Go. Exposes a REST surface that lets platform operators create, update, list, and delete Kubernetes resources across the company's three production clusters (EU-WEST, US-EAST, AP-SOUTHEAST) with role-based access control and audit logging. Deployed as a single binary in a distroless container, running as a non-root user with only the capabilities it actually needs. Handles ~800 RPS at peak with a p99 latency budget of 250ms end-to-end.

## Architecture

- `cmd/api/` — HTTP server entrypoint; wires middleware, DB connection, k8s client factory, and shutdown handler
- `cmd/migrator/` — One-shot binary that runs PostgreSQL migrations at deploy time; separate binary so it can run in an init container with narrower permissions
- `internal/handler/` — HTTP handlers, one file per resource type (deployment.go, namespace.go, configmap.go, secret.go, ingress.go); handlers are thin, delegating all business logic to the service layer
- `internal/service/` — Business logic layer; enforces authorization, orchestrates DB + k8s writes, emits audit events
- `internal/k8s/` — Kubernetes client wrappers that abstract context selection, retry policy, and error normalization
- `internal/auth/` — JWT validation and RBAC; resolves tokens to `(user_id, roles, scopes)` tuples
- `internal/store/` — PostgreSQL persistence via sqlc-generated code; transactional boundaries live here
- `internal/audit/` — Append-only audit log writer, backed by a partitioned PostgreSQL table
- `internal/reconciler/` — Background loop that reconciles PostgreSQL state with live Kubernetes state; runs every 30 seconds
- `internal/telemetry/` — OpenTelemetry setup, Prometheus metric registration, structured logging via slog

## Conventions

- All public functions take `context.Context` as first parameter
- Errors wrapped with `fmt.Errorf("failed to <verb>: %w", err)` — never silently drop, never re-format with %s
- HTTP handlers return JSON, never HTML; errors use RFC 9457 problem-details shape
- Kubernetes operations are idempotent — `apply` semantics, not `create` — because clients retry and we must not duplicate resources
- Database access goes through sqlc-generated functions; hand-written SQL is reviewed like migration code
- Interfaces are declared at the consumer, not the producer; `internal/service` defines the k8s interface it consumes, `internal/k8s` implements it concretely
- No `init()` functions outside `testdata` packages — they break testability and obscure startup order

## Data Model

PostgreSQL is the source of truth for resource *intent*. Kubernetes holds the observed runtime state. A reconciler loop closes the gap.

- `resources` — one row per managed Kubernetes resource; columns `(id, cluster, namespace, kind, name, spec_json, generation, created_at, updated_at)`; UNIQUE on `(cluster, namespace, kind, name)`
- `reconciliations` — append-only log of reconciler runs; `(resource_id, observed_generation, ran_at, ok, error)` — lets us detect drift
- `audit_log` — partitioned by month, 13 months retention; every mutating API call leaves a row with `(user_id, action, resource_ref, before_json, after_json, request_id, timestamp)`
- `idempotency_keys` — `(user_id, path, key, response_json, status_code, expires_at)`; 24h TTL; garbage collected by a nightly job
- `api_keys` — service-account tokens, scoped to a single cluster and a specific set of resource kinds
- `user_roles` — many-to-many between users and roles; roles are `admin`, `operator`, `viewer`

## Service Layer Contract

Every write goes through a fixed pipeline:

1. **Parse & validate** — handler decodes JSON, validator tags enforce basic shape, `DisallowUnknownFields` rejects typos
2. **Authorize** — service layer checks `(user, action, resource_kind, cluster)` against the RBAC matrix; returns 403 early if denied
3. **Persist intent** — write the new spec to PostgreSQL inside a transaction
4. **Apply to Kubernetes** — inside the same transaction, call the k8s client with apply semantics; on success, commit; on failure, rollback the PG write so the two stay consistent
5. **Audit** — after commit, write an audit-log row; audit failure is logged but does not fail the request (the mutation already happened)
6. **Respond** — serialize the authoritative state read back from PostgreSQL

## Request parsing

- Decode JSON with `json.NewDecoder(r.Body).Decode(&req)` — do not buffer the whole body unless you need to replay it
- Validate with go-playground/validator tags, return 400 with the first error as `{"error": "<field>: <message>"}`
- Reject unknown fields with `decoder.DisallowUnknownFields()` — silently ignoring a misspelled field is the kind of bug that costs a release

## Response shaping

- Always set `Content-Type: application/json` before writing the body
- Wrap list responses as `{"items": [...], "total": N, "next_page_token": "<opaque>"}` — never return a bare array; bare arrays are hard to extend with pagination metadata without breaking clients
- For create/update, respond 201/200 with the full resource, including server-assigned fields like `uid`, `created_at`, `generation`
- Errors follow RFC 9457 problem details: `{"type": "<uri>", "title": "...", "status": 4xx, "detail": "...", "instance": "/req/<id>"}`

## Authentication & RBAC

JWT tokens issued by the corporate SSO provider (Okta) carry `sub`, `email`, `groups`, and an `exp` no longer than 1 hour. A middleware validates the signature against a cached JWKS set, extracts the claims, maps `groups` to `roles` via the `group_role_mapping` table, and attaches a `*UserContext` to the request.

RBAC is defined in `internal/auth/policy.go` as a static matrix: `(role, resource_kind, action) -> allow|deny`. The matrix is hot-reloadable via SIGHUP for operational agility during incidents.

Service-to-service calls use API keys from the `api_keys` table instead of JWT; keys are rotated every 90 days and the grace window for old keys is 7 days.

## Kubernetes Client

The `internal/k8s` package abstracts multi-cluster access. A `ClientFactory` holds per-cluster `*kubernetes.Clientset` and `*dynamic.DynamicClient` instances, keyed by cluster ID. Context resolution:

1. Request carries `cluster` path or query parameter
2. Factory returns the matching client or 404 if cluster unknown
3. All calls use a derived context with a per-request 15s timeout

Retries use `k8s.io/client-go/util/retry.DefaultBackoff` which includes jitter. Custom retry loops are forbidden — we learned that lesson during incident PLAT-1903 when a custom loop synchronized across pods and caused a retry storm.

## Reconciliation Loop

Every 30 seconds, the reconciler:

1. Lists all `resources` rows with `updated_at > last_run - 5m` (covers the drift window)
2. For each, reads observed state from the relevant cluster via `k8s.Get()`
3. Computes a structural diff against `spec_json`
4. If drift detected, emits a Prometheus counter `reconciler_drift_total{cluster,kind}` and logs the diff at INFO
5. Does *not* auto-correct drift — operator decides whether to push intent or capture observed as new intent

## Observability

- **Metrics:** Prometheus exposition at `/metrics`; RED metrics per endpoint, plus business metrics (resource count by kind, reconciler drift rate)
- **Tracing:** OpenTelemetry, OTLP exporter pointed at `otel-collector.internal:4317`; 10% sampling in prod, 100% in staging
- **Logging:** `slog` with JSON handler in prod, text handler in dev; every log includes `request_id` and `user_id` when available; PII (JWT claims beyond `sub`) is never logged
- **Dashboards:** `grafana.internal/d/infra-api` (primary health), `grafana.internal/d/infra-api-reconciler` (drift and loop timing), `grafana.internal/d/slo-infra-api` (SLO burn rate)

## SLOs

- Availability: 99.9% of requests return a non-5xx status over a rolling 28-day window
- Latency: p99 < 250ms for read endpoints, p99 < 500ms for write endpoints
- Reconciler drift detection: p99 < 60s from mutation to drift-detected alert
- Error budget burn > 5%/hour sustained for 30 min triggers a page

## Deployment

- Built by Jenkins on every PR, published to the internal Harbor registry as `registry.internal/infra-api:<git-sha>`
- Deployed via ArgoCD from a GitOps repo; production is gated on staging soak time of 24 hours minimum
- Rolling update with `maxSurge: 1, maxUnavailable: 0`, pre-stop hook drains connections for 20s before SIGTERM
- Database migrations run in the migrator init container; rollback requires a matching down-migration

```bash
go build ./cmd/api/
go test ./...
go test -tags=integration ./internal/store/...
```

Integration tests require `TEST_DB_URL` and `TEST_K8S_KUBECONFIG` environment variables and a running `kind` cluster; the test harness seeds a minimal namespace layout into `kind` on first run.

## Testing Strategy

- **Unit tests:** every package has a `_test.go` file; target 80% line coverage on `internal/service` and `internal/auth`
- **Integration tests:** `_test.go` files tagged with `//go:build integration`; run against real PostgreSQL and kind; gate PR merge in CI
- **Contract tests:** `internal/k8s` has contract tests that run against both a fake client and a real kind cluster, ensuring parity
- **E2E tests:** a separate `e2e/` repo drives the full HTTP API against a staging instance; runs nightly and post-deploy
- **No snapshot tests** — reviewers can't tell a meaningful change from a formatting blip

## Secrets Management

HashiCorp Vault stores the JWKS signing keys, database credentials, and the service-account tokens used to talk to each cluster. The binary reads secrets at startup via the Vault Kubernetes auth method; tokens are renewed every 30 minutes by a background goroutine.

If Vault is unreachable at startup, the binary refuses to start (fail-closed). If Vault becomes unreachable mid-run, the binary continues serving with cached secrets until they expire — at which point requests begin to fail with 503. An alert fires on sustained renewal failures.

## API Versioning

URL-path versioning: `/v1/...`, `/v2/...`. Minor changes (additive fields, new optional parameters) go into the current major. Breaking changes require a new major version and a documented migration path; both majors run in parallel for at least 90 days before the older is deprecated and then removed 30 days after deprecation.

## Build and Test

```bash
go build ./cmd/api/
go test ./...
go test -tags=integration ./internal/store/...
golangci-lint run ./...
```

## Local Development

```bash
make dev-up           # starts postgres + vault + kind cluster via docker compose + kind
make seed             # loads demo resources, a test user, and a signing key pair
make dev              # runs the API against the local stack on :8080
make dev-integration  # runs the integration test suite against the local stack
```

The API runs behind an ingress controller. Health check at `/healthz` (unauthenticated, checks DB + k8s reachability), readiness at `/readyz` (stricter, returns 503 if any downstream is degraded).
