---
name: Architecture decisions
description: Key architectural decisions for the infra-api project with rationale
type: project
---

# Architecture Decisions

## Use service layer between handlers and k8s client

All Kubernetes operations go through `internal/service/`, never called directly from HTTP handlers.

**Why:** Direct handler-to-k8s coupling made it impossible to add audit logging and RBAC checks without duplicating code across every handler.

**How to apply:** Handlers validate input and call service methods. Services enforce authorization, perform the operation, and emit audit events.

## Idempotent apply semantics for all mutations

Every create/update endpoint uses `apply` semantics — if the resource exists, update it; if not, create it.

**Why:** Clients retry on network errors. Non-idempotent `create` endpoints caused duplicate resources during retries and required client-side deduplication logic.

**How to apply:** Service methods call `k8s.Apply()` not `k8s.Create()`. Response includes whether the resource was created or updated.

## PostgreSQL for state, Kubernetes for runtime

Resource definitions live in PostgreSQL. Kubernetes is the execution layer only.

**Why:** Kubernetes etcd is not designed for complex queries. Listing resources with filters, pagination, and sorting requires a relational database.

**How to apply:** All reads go through PostgreSQL. Writes go to both PostgreSQL (source of truth) and Kubernetes (desired state). A reconciler loop ensures convergence.
