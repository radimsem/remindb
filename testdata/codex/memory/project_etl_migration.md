---
name: ETL migration to Airflow 2.9
description: Migration progress from legacy cron jobs to Airflow DAGs
type: project
---

# ETL Migration Progress

## Status (2026-04-14)

Migrating 12 legacy cron-based ETL jobs to Airflow DAGs. 8 of 12 completed and running in production.

## Completed

- Vendor A daily extract (JIRA: DATA-401)
- Vendor B hourly metrics (JIRA: DATA-402)
- Customer segmentation transform (JIRA: DATA-405)
- Revenue aggregation pipeline (JIRA: DATA-406)
- Inventory sync from warehouse API (JIRA: DATA-410)
- Marketing attribution model refresh (JIRA: DATA-411)
- User activity rollup (JIRA: DATA-413)
- Product catalog enrichment (JIRA: DATA-414)

## Remaining

- Vendor C real-time events (JIRA: DATA-403) — blocked on WebSocket operator implementation
- Fraud detection scoring (JIRA: DATA-407) — requires ML model serving integration with SageMaker
- Geographic demand forecasting (JIRA: DATA-408) — depends on new Snowflake UDF deployment
- Partner settlement reconciliation (JIRA: DATA-412) — waiting for finance team schema approval

**Why this order:** Revenue-impacting pipelines migrated first. Remaining four have external dependencies that need coordination.

**How to apply:** Do not start DATA-403, DATA-407, DATA-408, or DATA-412 without checking the blocker status first. Each has a linked JIRA ticket with the dependency chain.
