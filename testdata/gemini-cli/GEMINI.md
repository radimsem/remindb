---
project: infra-api
language: go
runtime: kubernetes
---

# Project Instructions

## Overview

Infrastructure management API written in Go. Manages Kubernetes resources through a REST API with role-based access control. Deployed as a single binary in a container.

## Architecture

- `cmd/api/` — HTTP server entrypoint
- `internal/handler/` — HTTP handlers, one file per resource type
- `internal/service/` — Business logic layer
- `internal/k8s/` — Kubernetes client wrappers
- `internal/auth/` — JWT validation and RBAC
- `internal/store/` — PostgreSQL persistence

## Conventions

- All public functions take `context.Context` as first parameter
- Errors wrapped with `fmt.Errorf("failed to <verb>: %w", err)`
- HTTP handlers return JSON, never HTML
- Kubernetes operations are idempotent — `apply` semantics, not `create`

## Build and Test

```bash
go build ./cmd/api/
go test ./...
go test -tags=integration ./internal/store/...
```

Integration tests require `TEST_DB_URL` and `TEST_K8S_KUBECONFIG` environment variables.

## Deployment

```bash
docker build -t infra-api .
kubectl apply -f deploy/
```

The API runs behind an ingress controller. Health check at `/healthz`, readiness at `/readyz`.
