---
name: Monitoring and dashboard references
description: Where to find dashboards, logs, and lineage for dataflow pipelines
type: reference
---

# Monitoring

## Airflow

- Web UI: https://airflow.internal.acme.com
- DAG list (filtered to production): https://airflow.internal.acme.com/dags?tags=prod
- Scheduler logs (via k8s): `kubectl logs -n airflow -l app=airflow-scheduler --tail=500`
- Task log persistence: S3 at `s3://acme-airflow-logs/<dag_id>/<task_id>/<run_id>/`

## Snowflake

- Query history UI: https://app.snowflake.com/acme-prod/usage
- Credit consumption dashboard: https://app.snowflake.com/acme-prod/finops
- Query lineage (for ETL troubleshooting): https://datahub.internal/search?entity=Dataset&platform=snowflake
- Warehouse auto-suspend: `ETL_WH` suspends after 60s idle; `ANALYTICS_WH` after 300s

## Grafana

- ETL health overview: https://grafana.internal/d/etl-health
- Per-vendor extraction rates: https://grafana.internal/d/vendor-extraction
- Dead letter queue depth: https://grafana.internal/d/dlq — alerts fire at sustained DLQ > 100 records for 15 min

## Logs and alerts

- Structured log search (Loki): https://grafana.internal/explore?datasource=loki&queries=%7Bjob%3D%22airflow%22%7D
- PagerDuty service: `dataflow-oncall`
- Slack alert channels:
  - `#data-alerts` — SLA misses, DLQ threshold breaches
  - `#data-incidents` — active incidents only, lower noise
- Snowflake query alerts: configured in Snowsight as resource monitors; notifications to `#data-alerts`
