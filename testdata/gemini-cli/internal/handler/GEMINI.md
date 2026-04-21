---
scope: handlers
inherits: ../../GEMINI.md
---

# HTTP Handler Conventions

All files in this directory are HTTP handlers. One resource type per file (namespace.go, deployment.go, configmap.go). Handlers are thin — they validate input, call a service method, and shape the response. Business logic belongs in `internal/service/`, never here.

## Request parsing

- Decode JSON with `json.NewDecoder(r.Body).Decode(&req)` — do not buffer the whole body unless you need to replay it
- Validate with go-playground/validator tags, return 400 with the first error as `{"error": "<field>: <message>"}`
- Reject unknown fields with `decoder.DisallowUnknownFields()` — silently ignoring a misspelled field is the kind of bug that costs a release

## Response shaping

- Always set `Content-Type: application/json` before writing
- Wrap list responses as `{"items": [...], "total": N}` — do not return a bare array (harder to extend with pagination metadata)
- For create/update, respond 201/200 with the full resource, including server-assigned fields like `uid`, `createdAt`

## Error mapping

| service error          | HTTP status | payload                              |
|------------------------|-------------|--------------------------------------|
| `service.ErrNotFound`  | 404         | `{"error": "<resource> not found"}`  |
| `service.ErrConflict`  | 409         | `{"error": "<reason>"}`              |
| `service.ErrForbidden` | 403         | `{"error": "forbidden"}`             |
| any other              | 500         | `{"error": "internal server error"}` with full error logged server-side |

Never leak the underlying error string to the client — it may contain Kubernetes API internals or PostgreSQL table names.

## Idempotency

Mutating endpoints (POST, PUT, PATCH) honor an `Idempotency-Key` header when present:

1. Look up the key in `idempotency_keys` table (scoped by user + resource path)
2. If found, return the cached response verbatim
3. If not found, process the request, then cache the response keyed by (user, path, Idempotency-Key) with a 24h TTL
