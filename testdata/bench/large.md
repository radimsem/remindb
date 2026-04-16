---
name: Platform Architecture
description: Comprehensive design document for the distributed platform
type: project
---

# Platform Architecture

This document captures the full architecture of the distributed platform, covering
compute, storage, networking, security, observability, and deployment.

## Design Principles

The platform follows five core principles:

- Loose coupling between services via async messaging
- Data ownership per service boundary
- Progressive rollout for all changes
- Observability as a first-class concern
- Graceful degradation over hard failures

## Service Boundaries

Services are split along business domain lines. Each service owns its data store
and exposes a well-defined API contract. Cross-service queries go through published
APIs, never direct database access.

# Compute Layer

The compute layer runs all application workloads on Kubernetes with auto-scaling
based on request rate and queue depth.

## Container Runtime

### Base Images

All services use a multi-stage build with a distroless runtime image.
Base images are rebuilt weekly with security patches.
Image scanning runs in CI; critical CVEs block deployment.

### Resource Allocation

```yaml
profiles:
  api:
    requests: {cpu: 250m, memory: 128Mi}
    limits: {cpu: 1000m, memory: 512Mi}
  worker:
    requests: {cpu: 500m, memory: 256Mi}
    limits: {cpu: 2000m, memory: 1Gi}
  batch:
    requests: {cpu: 1000m, memory: 512Mi}
    limits: {cpu: 4000m, memory: 4Gi}
```

### Health Checks

Every container exposes three probe endpoints:

- `/healthz` — liveness, restarts on failure
- `/readyz` — readiness, removes from load balancer
- `/startupz` — startup, allows slow initialization

Liveness checks run every 10 seconds with a 3-second timeout.
Readiness checks run every 5 seconds. Startup probes allow up to 120 seconds.

## Auto-Scaling

### Horizontal Pod Autoscaler

HPA targets 70% CPU utilization with a 30-second stabilization window.
Minimum replicas: 2 for all services, 5 for the API gateway.
Maximum replicas: 50 for most services, 100 for the search service.

### Vertical Pod Autoscaler

VPA runs in recommendation-only mode. Monthly reviews of recommendations
inform resource request adjustments. Auto-update mode is disabled to prevent
unexpected restarts.

### KEDA Event-Driven Scaling

Queue-based workers scale on queue depth using KEDA.
Threshold: 1 pod per 100 pending messages. Cooldown period: 5 minutes.

## Service Mesh

### Istio Configuration

Istio handles mTLS, traffic splitting, and retry policies between services.
All inter-service traffic is encrypted with mTLS certificates rotated every 24 hours.

### Traffic Management

Canary deployments use Istio virtual services with progressive traffic shifting:
5% → 25% → 50% → 100% over 4 hours. Automatic rollback triggers on:

- Error rate > 1% for the canary
- p99 latency > 2x baseline
- Any 5xx spike > 5 per minute

### Fault Injection

Chaos testing injects faults weekly:

- 5% random HTTP 500 responses between services
- 100ms latency injection on 10% of requests
- Pod termination of random instances

# Storage Layer

## Primary Databases

### PostgreSQL Cluster

PostgreSQL 16 with streaming replication. Primary in us-east-1a, synchronous
replica in us-east-1b, async replica in us-west-2 for disaster recovery.

Connection pooling via PgBouncer with 200 max connections per pool.
Statement-level pooling for most services, transaction-level for long-running workers.
The pool is monitored by a dedicated exporter that tracks active connections, queued
requests, and average wait time. Alerts fire when queue depth exceeds 50 or average
wait time exceeds 10ms, indicating either a connection leak or a capacity issue.
During peak traffic, the pool temporarily expands to 300 connections via an auto-scaling
policy that watches the queue depth metric over a 30-second window.

#### Schema Management

Migrations run via golang-migrate. Every migration must be reversible.
Breaking schema changes follow expand-contract pattern:

1. Add new column (nullable or with default)
2. Deploy code that writes to both old and new columns
3. Backfill new column
4. Deploy code that reads from new column
5. Remove old column

#### Query Performance

Slow query threshold: 100ms. Queries exceeding threshold are logged with
full query plan to a dedicated slow_queries table and tracked in a Grafana
dashboard. Weekly review of top 10 slow queries by cumulative wall-clock time.
The review process prioritizes queries that regressed since the last release
over historically slow queries that have stable performance. Common remediations
include adding covering indexes, rewriting correlated subqueries as joins,
and adjusting work_mem for sorting-heavy queries.

Index analysis runs monthly:

```sql
SELECT schemaname, relname, idx_scan, idx_tup_read, idx_tup_fetch
FROM pg_stat_user_indexes
WHERE idx_scan = 0
ORDER BY pg_relation_size(indexrelid) DESC
LIMIT 20;
```

### ClickHouse Analytics

ClickHouse handles all analytical queries. Data flows from PostgreSQL
via Debezium CDC into Kafka, then into ClickHouse via a custom consumer.
The consumer runs as a Kubernetes deployment with 3 replicas, each processing
a subset of Kafka partitions. Exactly-once semantics are achieved by storing
Kafka offsets in ClickHouse alongside the data in the same transaction.
Backfill jobs run on a separate consumer group that reads from the earliest
offset and writes to a staging table, which is then swapped into production
via EXCHANGE TABLES after validation.

Materialized views pre-aggregate common metrics:

- Daily active users by cohort
- Revenue by product line and region
- API usage by client and endpoint
- Error rates by service and error code

#### Partition Strategy

Tables are partitioned by month. Partitions older than 2 years are moved
to S3-backed cold storage via a nightly job that runs ALTER TABLE MOVE PARTITION
and verifies row counts before dropping the local copy. TTL policies auto-expire
raw event data after 90 days, while aggregated materialized view data is retained
indefinitely. Partition pruning is critical for query performance — queries that
do not include a date range filter are rejected by a query complexity guard that
checks the estimated rows scanned before execution. The guard threshold is set
to 10 billion rows, above which queries must be pre-approved by the data team.

## Cache Layer

### Redis Cluster

Redis 7 cluster with 6 nodes (3 primary, 3 replica). Memory limit: 64GB per node.
Eviction policy: allkeys-lfu for session caches, volatile-ttl for rate limit counters.

### Cache Patterns

```go
func (s *Service) GetUser(ctx context.Context, id string) (*User, error) {
    key := "user:" + id
    if cached, err := s.cache.Get(ctx, key); err == nil {
        var u User
        if json.Unmarshal(cached, &u) == nil {
            return &u, nil
        }
    }

    u, err := s.db.GetUser(ctx, id)
    if err != nil {
        return nil, err
    }

    data, _ := json.Marshal(u)
    s.cache.Set(ctx, key, data, 15*time.Minute)
    return u, nil
}
```

Cache invalidation uses pub/sub. On write, the owning service publishes
an invalidation event. Subscribers evict the stale key. Eventual consistency
window: typically under 50ms within the same region.

### Cache Stampede Prevention

Singleflight groups prevent cache stampedes on popular keys.
For less critical data, stale-while-revalidate with a 30-second grace period.

## Object Storage

### S3 Configuration

All binary assets (images, documents, exports) stored in S3 with server-side
encryption (SSE-S3). Lifecycle policies:

| Bucket | Transition to IA | Transition to Glacier | Expire |
|--------|-----------------|----------------------|--------|
| uploads | 30 days | 90 days | 365 days |
| exports | 7 days | 30 days | 90 days |
| backups | 60 days | 180 days | never |
| logs | 14 days | 60 days | 365 days |

### CDN Integration

CloudFront distribution serves static assets with 24-hour TTL.
Cache invalidation runs on deploy via CI. Origin failover to secondary
S3 bucket in us-west-2.

# Messaging Layer

## Kafka Cluster

### Cluster Topology

Kafka 3.7 with 9 brokers across 3 availability zones. Replication factor: 3.
Min in-sync replicas: 2. Unclean leader election: disabled.

### Topic Design

Topics follow the pattern: `{domain}.{entity}.{event-type}`.
Examples:

- `orders.order.created`
- `orders.order.completed`
- `users.profile.updated`
- `payments.transaction.failed`
- `inventory.stock.adjusted`

Partitioning strategy: hash on entity ID for ordering guarantees within
a single entity. Default partition count: 12 for most topics, 48 for high-throughput
topics like analytics events.

### Consumer Groups

Each service has exactly one consumer group per topic it subscribes to.
Consumer lag alerting triggers at 10,000 messages. Processing guarantee:
at-least-once delivery with idempotent consumers.

### Dead Letter Queues

Failed messages retry 3 times with exponential backoff (1s, 5s, 25s).
After exhausting retries, messages move to a dead letter topic.
DLQ dashboard shows pending items; on-call reviews daily.

## Event Schema Registry

Confluent Schema Registry enforces Avro schemas for all Kafka topics.
Schema evolution rules: backward-compatible changes only (add optional fields,
remove optional fields with defaults). Breaking changes require a new topic version.

# Security

## Network Security

### VPC Architecture

Production runs in a dedicated VPC with three subnet tiers:

- Public subnets: load balancers only
- Private subnets: application workloads
- Isolated subnets: databases, caches

Security groups enforce least-privilege access. No direct internet access
from private or isolated subnets. NAT gateways handle outbound traffic.

### Network Policies

Kubernetes network policies restrict pod-to-pod traffic.
Default deny all ingress. Explicit allow rules for each service dependency.

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-gateway-to-user-service
spec:
  podSelector:
    matchLabels:
      app: user-service
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app: gateway
      ports:
        - port: 8080
          protocol: TCP
```

## Secret Management

### Vault Integration

HashiCorp Vault stores all secrets. Services authenticate via Kubernetes
service account tokens. Secret rotation:

- Database credentials: every 24 hours
- API keys: every 90 days
- TLS certificates: every 30 days
- Encryption keys: every 365 days

### Secret Injection

Vault Agent sidecar injects secrets as files in the pod filesystem.
Environment variable injection is prohibited — secrets in env vars
leak into crash dumps, logs, and child processes.

## Authentication and Authorization

### RBAC Model

Role-based access control with four predefined roles:

- `viewer` — read-only access to owned resources
- `editor` — create and modify owned resources
- `admin` — full access within organization
- `superadmin` — platform-level access (internal only)

Custom roles supported for enterprise customers. Permissions are additive;
deny rules are not supported to keep the model simple and auditable.

### Audit Logging

All authentication events and authorization decisions are logged.
Audit logs are immutable, stored in a separate ClickHouse cluster.
Retention: 7 years for compliance. Query access restricted to security team.

# Observability

## Metrics

### Prometheus Stack

Prometheus scrapes all services every 15 seconds.
Long-term storage via Thanos with S3 backend. Retention:

- Raw metrics: 15 days (local)
- Downsampled 5m: 90 days (Thanos)
- Downsampled 1h: 2 years (Thanos)

### Standard Metrics

Every service must expose:

```
# Request metrics
http_requests_total{method, path, status}
http_request_duration_seconds{method, path}

# Runtime metrics
go_goroutines
go_memstats_alloc_bytes
process_cpu_seconds_total

# Business metrics (service-specific)
orders_created_total{type}
payments_processed_total{provider, status}
```

### Alerting Rules

Critical alerts page on-call via PagerDuty. Warning alerts go to Slack.

| Alert | Condition | Severity |
|-------|-----------|----------|
| High error rate | 5xx > 1% for 5 min | critical |
| High latency | p99 > 2s for 5 min | critical |
| Pod crash loop | restarts > 5 in 10 min | critical |
| Disk usage | > 80% | warning |
| Certificate expiry | < 14 days | warning |
| Consumer lag | > 50k messages | warning |
| Memory pressure | > 90% limit | warning |

## Distributed Tracing

### OpenTelemetry Integration

All services instrument with OpenTelemetry SDK. Traces export to Tempo
via OTLP gRPC. Sampling strategy: head-based at 10%, tail-based at 100%
for errors and slow requests (> 1s).

### Trace Context Propagation

W3C Trace Context headers propagate through HTTP, gRPC, and Kafka.
Custom baggage carries tenant ID and feature flags for context-aware routing.

### Span Conventions

```go
func (s *Service) ProcessOrder(ctx context.Context, order *Order) error {
    ctx, span := tracer.Start(ctx, "ProcessOrder",
        trace.WithAttributes(
            attribute.String("order.id", order.ID),
            attribute.String("order.type", order.Type),
        ),
    )
    defer span.End()

    if err := s.validate(ctx, order); err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, "validation failed")
        return err
    }

    return s.repository.Save(ctx, order)
}
```

## Logging

### Structured Logging

All logs are JSON-formatted with standard fields:

- `timestamp` — RFC3339Nano
- `level` — debug, info, warn, error
- `message` — human-readable description
- `service` — service name
- `trace_id` — OpenTelemetry trace ID
- `span_id` — OpenTelemetry span ID
- `request_id` — unique request identifier

### Log Aggregation

Fluentd collects container logs and ships to OpenSearch.
Index-per-day with 30-day retention. Hot-warm-cold architecture:

- Hot: 3 days on NVMe SSD
- Warm: 14 days on standard SSD
- Cold: 30 days on HDD with force-merge

# Deployment Pipeline

## CI/CD

### Pipeline Stages

1. **Lint** — golangci-lint, eslint, hadolint for Dockerfiles
2. **Test** — unit tests, integration tests with testcontainers
3. **Build** — multi-stage Docker build, SBOM generation
4. **Scan** — Trivy image scan, Snyk dependency scan
5. **Push** — push to ECR with immutable tags
6. **Deploy** — ArgoCD syncs Kubernetes manifests
7. **Verify** — smoke tests against canary pods

### Branch Strategy

Trunk-based development with short-lived feature branches.
PRs require 1 approval and passing CI. Merge commits disabled;
squash merge only. Main branch deploys to staging automatically.

### Feature Flags

LaunchDarkly manages feature flags. New features deploy behind flags.
Flag lifecycle:

- Created: at feature branch start
- Enabled: progressive rollout in production
- Removed: 2 sprints after full rollout (mandatory cleanup)

## Infrastructure as Code

### Terraform Modules

All infrastructure is managed via Terraform with remote state in S3.
Module structure:

- `modules/vpc` — VPC, subnets, NAT gateways
- `modules/eks` — EKS cluster, node groups, addons
- `modules/rds` — PostgreSQL instances, parameter groups
- `modules/redis` — ElastiCache Redis clusters
- `modules/kafka` — MSK cluster configuration
- `modules/monitoring` — Prometheus, Grafana, alerting

### Drift Detection

Weekly Terraform plan runs detect configuration drift.
Drift alerts go to the infrastructure Slack channel.
Manual changes to production infrastructure are prohibited;
all changes must go through the Terraform pipeline.

## Disaster Recovery

### RPO and RTO Targets

| Component | RPO | RTO |
|-----------|-----|-----|
| PostgreSQL | 1 minute | 15 minutes |
| Redis | 5 minutes | 5 minutes |
| Kafka | 0 (replicated) | 10 minutes |
| S3 | 0 (replicated) | 0 (multi-AZ) |
| Application | N/A | 5 minutes |

### Failover Procedures

Database failover is automatic via RDS multi-AZ. Application failover
uses Route 53 health checks with 30-second TTL. Full region failover
is manual, triggered by the incident commander, targeting 30-minute RTO.

### Backup Strategy

PostgreSQL: continuous WAL archiving to S3, daily base backups.
Point-in-time recovery tested monthly. Backup verification runs nightly:
restore to a temporary instance, run consistency checks, tear down.

Redis: AOF persistence with 1-second fsync. RDB snapshots every 6 hours.
Snapshots replicated to secondary region.

## Incident Response

### Severity Levels

- **SEV1** — complete outage or data loss. All hands. 15-minute response.
- **SEV2** — major feature degraded. On-call + team lead. 30-minute response.
- **SEV3** — minor feature degraded. On-call only. 2-hour response.
- **SEV4** — cosmetic or non-impacting. Next business day.

### Runbooks

Every alert links to a runbook. Runbook structure:

1. **Symptoms** — what the alert looks like
2. **Impact** — what users experience
3. **Investigation** — step-by-step diagnostic commands
4. **Mitigation** — immediate actions to reduce impact
5. **Resolution** — permanent fix procedures
6. **Prevention** — follow-up tasks to prevent recurrence

### Post-Incident Reviews

Blameless post-mortems within 48 hours of SEV1/SEV2 resolution.
Action items tracked in Linear with due dates. Review completion
rate is a team KPI.
