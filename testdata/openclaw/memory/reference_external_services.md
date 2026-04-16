---
agent: atlas
type: reference
created: 2026-04-10
---

# External Service References

## Documentation

- API rate limiting design doc: `docs.meridian.internal/api/rate-limiting`
- Auth middleware RFC: `docs.meridian.internal/rfc/structured-claims`
- Infrastructure runbook: `docs.meridian.internal/ops/runbook`

## Dashboards

- Sentry project: `sentry.meridian.internal/projects/api-gateway`
- Grafana API latency: `grafana.meridian.internal/d/api-latency`
- CI pipeline status: `github.com/meridian-labs/api-gateway/actions`

## Infrastructure

- S3 bucket: `meridian-assets-prod` in eu-west-1 (migrated from us-east-1 on 2026-04-09)
- PostgreSQL: RDS cluster `meridian-prod-pg15` in eu-west-1
- Redis: ElastiCache cluster `meridian-cache-prod` for session storage
