---
project: dataflow
framework: airflow
language: python
version: "3.12"
---

# Project Overview

## Architecture

Dataflow is an ETL pipeline platform built on Apache Airflow 2.9 running on Amazon EKS. It ingests data from 14 vendor APIs and 3 internal services, transforms it through pandas and dbt, and loads into a Snowflake warehouse partitioned into raw, staging, and mart layers. The platform processes roughly 2.4 TB of new data per day and serves 60+ downstream analytics consumers across product, finance, marketing, and data-science teams.

### Directory Structure

- `dags/` — Airflow DAG definitions, one file per pipeline; grouped under `dags/vendors/`, `dags/internal/`, `dags/backfills/`, and `dags/dbt/`
- `operators/` — Custom Airflow operators for vendor integrations; each subclasses `BaseExtractor` from `operators/base.py`
- `transforms/` — Pure transformation functions, no I/O; these are the easiest code to unit-test because they take DataFrames in and return DataFrames out
- `loaders/` — Snowflake and S3 write logic; the S3 writer hashes payloads and dedupes before COPY INTO
- `schemas/` — Pydantic models for API responses and intermediate data; live at the boundary between untrusted external data and trusted internal processing
- `dbt_project/` — dbt models for staging → mart transformations, materialized as Snowflake views or tables depending on size and query frequency
- `tests/` — Pytest tests mirroring source structure; `tests/dags/` has DAG-parse smoke tests, `tests/transforms/` has the dense coverage
- `plugins/` — Airflow plugins: custom sensors, macros, and the in-house `DataContractChecker` operator
- `config/` — Environment-specific YAML loaded at Airflow import time; dev, staging, prod each have their own file

### Data Flow

1. Extractors pull raw JSON from vendor APIs using httpx with retry; responses are cached in S3 raw bucket with content-addressed keys so re-runs are free
2. Validators parse responses through Pydantic models, rejecting malformed records to a per-pipeline dead letter queue rather than raising
3. Transformers apply business logic as pure functions on DataFrames; transforms never call out to external services
4. Loaders write parquet files to an S3 staging bucket, then issue `COPY INTO` against Snowflake's external stage; idempotent on the partition key
5. dbt models run after raw-layer loads complete, materializing staging tables and downstream marts; dependencies expressed via `ref()`

## Conventions

### Type Annotations

All functions must have type annotations. Use `typing.Protocol` for dependency injection boundaries rather than inheritance. Avoid `Any` — prefer `object` or explicit union types. Runtime type checking via `typeguard` is enabled in the test suite but disabled in production for performance. The mypy config is strict: no implicit optional, no untyped defs, no untyped calls.

### Error Handling

- Extractors retry transient HTTP errors (429, 502, 503) with exponential backoff and jitter; retries cap at 5 attempts, then the task fails and the DAG run is marked failed
- Validators log rejected records to a dead letter queue S3 prefix, keyed by DAG run ID; never raise on bad data — one malformed row should never fail the whole batch
- Transformers raise `ValueError` on invariant violations — these are bugs in our code or upstream schema changes, not data issues, and should wake someone up
- Loaders are idempotent: re-running a load for the same partition overwrites cleanly; staging parquet files are versioned by DAG run ID
- Every task emits structured logs including the DAG ID, task ID, run ID, and any vendor-specific correlation ID so cross-system debugging is traceable

### Testing

- Unit tests for transforms: pure input/output, no mocking needed; target 95% coverage on `transforms/`
- Integration tests for extractors: use `respx` to mock HTTP, never hit real vendor APIs; fixtures recorded with VCR-style replay
- DAG validation tests: verify all DAGs parse without import errors; catch circular dependencies and missing connections at CI time
- dbt tests: `dbt test` runs on every PR against a PR-scoped Snowflake schema created by a pre-test hook
- No snapshot tests — they broke on every pandas version bump and created review noise without catching real bugs

## Vendor Integration Pattern

Every vendor extractor extends `operators.base.BaseExtractor` which provides retry, pagination, auth refresh, and rate limiting out of the box. Concrete extractors implement three methods:

- `auth() -> httpx.Auth` — returns the vendor-appropriate auth mechanism, pulling credentials from Airflow Connections (never from environment or secrets files directly)
- `pages(since: datetime) -> Iterator[dict]` — yields one response page at a time; the base class handles pagination bookkeeping
- `normalize(page: dict) -> list[Record]` — converts raw response shape into our canonical `Record` dataclass

Adding a new vendor is a half-day task: stub the class, fill in the three methods, register the operator in `plugins/__init__.py`, and add a DAG under `dags/vendors/`. Integration tests must cover the auth refresh path and at least one pagination boundary.

## Snowflake Schema Conventions

- **Raw layer** (`RAW.<vendor>_<stream>`): one-to-one with source data, loaded as-is without transformation, partitioned by `extracted_at` date; retained for 90 days
- **Staging layer** (`STAGING.<domain>_<entity>`): deduplicated, typed, and normalized; this is where dbt does the heavy lifting
- **Mart layer** (`MART.<consumer>_<subject>`): consumer-facing tables optimized for analytical queries; typically wide with denormalized dimensions

Naming: lowercase with underscores; no camelCase, no hyphens. Date columns end in `_at` (timestamps) or `_date` (dates). Money columns always carry a `_cents` or `_micros` suffix and live as integers.

## Backfill Procedures

Backfills are manually triggered via the Airflow UI, never scheduled. The `dags/backfills/` directory holds backfill DAGs parameterized by date range; they inherit `max_active_runs=1` so only one backfill of a given pipeline runs at a time. Backfill compute uses a dedicated Snowflake warehouse (`BACKFILL_WH`) to avoid contending with production queries.

Before triggering a backfill, confirm with the pipeline owner that the vendor can handle the historical request volume. Some vendors rate-limit historical endpoints aggressively.

## Data Quality

Every pipeline emits row counts at each stage (raw, staging, mart) to a Prometheus pushgateway, and a dashboard tracks historical trends. Drops of more than 10% vs the prior 7-day average trigger a soft alert in `#data-alerts`. Drops of more than 25% trigger a hard page.

Schema validation runs on every batch via Pydantic; failures land in the dead letter queue. DLQ depth is monitored; sustained DLQ > 100 records over 15 min pages oncall.

## Dependency Management

Poetry manages Python dependencies. The lock file is committed and enforced in CI (`poetry check --lock`). Airflow plugin version compatibility is delicate — bumping Airflow itself is a multi-week project that requires coordinated plugin upgrades, so we pin Airflow exactly and review the compatibility matrix for every plugin upgrade.

Python 3.12 is the runtime. 3.13 upgrade is tracked in JIRA but not a 2026 goal.

## Environments

- **Dev:** each engineer runs a local docker-compose stack with PostgreSQL, Redis, and LocalStack for S3; Snowflake is shared (a `DEV` database with per-engineer schemas to avoid collisions)
- **Staging:** full mirror of prod, running on a smaller EKS node group; refreshed from prod every Monday morning via a sanitized dump
- **Prod:** the real thing; deploys are staged through staging with a 24-hour soak time unless it's a hotfix

## Observability

- **DAG runs:** Airflow metadata DB exposes state via the built-in `/health` endpoint; Prometheus scrapes the metadata DB directly
- **Task metrics:** custom StatsD-style metrics emitted per task: duration, input rows, output rows, DLQ rows
- **Snowflake query cost:** weekly report pulled from `SNOWFLAKE.ACCOUNT_USAGE.QUERY_HISTORY`; the top-10 most expensive queries are reviewed at the Monday engineering sync
- **Data lineage:** OpenLineage events emitted by a custom plugin to Marquez, which feeds the lineage UI the DS team uses for impact analysis

## Incident Response

DAG failures are classified by a tag:

- `severity:high` — customer-facing data downstream; pages oncall immediately
- `severity:medium` — internal-only data; Slack alert, next-business-day follow-up
- `severity:low` — best-effort or experimental pipelines; Slack alert only

SLA misses escalate independently of task failures: a pipeline missing its SLA 3 days in a row pages the owner regardless of whether tasks technically passed.

## Data Contracts

Between teams, every inbound data source has a contract stored in `contracts/<source>.yaml`: schema, freshness expectation, ownership, and version history. Schema changes on the producer side require a 14-day advance notice to consumers. The `DataContractChecker` plugin runs on every DAG run and fails the run if observed schema drifts from the contract.

## Cost Management

- Snowflake warehouses auto-suspend after 60s idle (`ETL_WH`) or 300s (`ANALYTICS_WH`); resizing requires an RFC reviewed at the Monday engineering sync
- S3 lifecycle policies move raw data to Glacier after 90 days and delete after 13 months unless tagged `retention=permanent`
- EKS right-sizing is reviewed quarterly; the scheduler uses spot instances for backfill workers

## Security

- Vendor credentials live in Airflow Connections, encrypted by Fernet at rest; the Fernet key is stored in AWS Secrets Manager and loaded at scheduler startup
- PII fields (email, phone, address, government IDs) are hashed before leaving the raw layer; the hash uses a salt rotated quarterly
- Snowflake access is role-based: `ETL_LOADER` can write, `ANALYST` can read, and `MART_CONSUMER` can read only mart-layer tables
- Audit logs from Snowflake `ACCESS_HISTORY` are forwarded to the security SIEM for anomaly detection

## Performance

- DAG parse time cap: 10 seconds per DAG file; the scheduler enforces this and logs offenders
- Task queue depth SLO: < 100 queued tasks for more than 5 minutes triggers a scale-up of worker pods
- Worker autoscaling: 6–60 pods based on queue depth, scaling cooldown 2 minutes to avoid flapping

## Release Process

- DAG code changes: standard PR flow, merge to main deploys to staging automatically, promotion to prod requires explicit approval after 24h soak
- Airflow upgrades: quarterly, tracked as a multi-PR migration with explicit rollback plan
- Plugin upgrades: each one is its own PR, tested against a scratch staging environment before landing

## Local Development

```bash
make dev-up          # brings up postgres + redis + localstack
make dev-airflow     # starts airflow scheduler + webserver on :8080
make dev-snowflake   # creates per-engineer Snowflake schema, prints connection info
make seed            # loads fixture data for one full DAG run
make test            # pytest everything, target < 2 minutes
```

## On-call

Weekly rotation, Monday handoff at 10:00 UTC. The outgoing oncall posts a summary of open pages, DLQ depth trends, and any deferred investigations in `#data-oncall`. Handoff doc template lives at `docs/oncall-handoff-template.md`.
