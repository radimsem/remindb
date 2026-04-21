---
name: Idempotency enforcement feedback
description: Platform team correction on how mutating Kubernetes operations must be written
type: feedback
---

Every mutating Kubernetes operation must be idempotent — use `apply` semantics, never `create`. If a resource exists, update it; if not, create it. Clients retry, and non-idempotent create endpoints produce duplicates during the retry.

**Why:** Confirmed after incident PLAT-1903 on 2026-04-02. The kubernetes client retry storm on 429 responses created N duplicate ConfigMaps because the create endpoint used plain `Create`, not `Apply`. Required a 4-hour reconciler job to fix.

**How to apply:** Service methods call `k8s.Apply()` with the resource manifest as the argument. The response distinguishes created vs updated via a `created bool` return. Handlers translate `created == true` to 201 Created, `created == false` to 200 OK.

---

Retries with exponential backoff and jitter, never without jitter. A thundering herd of synchronized retries is worse than the original failure.

**Why:** Same incident — the first retry storm synchronized because every client used the same base delay.

**How to apply:** Use `k8s.io/client-go/util/retry.DefaultBackoff` which already includes jitter. Do not implement custom retry loops; if you need custom behavior, extend DefaultBackoff by copying it and adjusting, not by writing from scratch.
