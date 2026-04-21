---
description: Scaffold a new backfill DAG for a vendor pipeline
argument-hint: "<vendor-name> <start-date> <end-date>"
---

Scaffold a backfill DAG for vendor `$1` covering dates `$2` through `$3`.

## Steps

1. Read `operators/base.py` and the existing vendor extractor at `operators/${1}_extract.py` to understand the vendor's auth and pagination model.
2. Create `dags/backfill_${1}_${2}_${3}.py`:
   - `start_date` and `end_date` from `$2` and `$3`
   - `schedule_interval=None` (manual trigger only)
   - `max_active_runs=1` so backfills don't stomp on each other
   - `default_args` with `retries=5` and `retry_exponential_backoff=True` — backfills are long and transient failures are expensive to re-run from scratch
   - Use the existing vendor extractor operator; do not duplicate its logic
3. Write a smoke test at `tests/dags/test_backfill_${1}.py` that verifies the DAG imports, has the expected task IDs, and the schedule is set to None.
4. Run `pytest -x tests/dags/test_backfill_${1}.py` and report the result.

## Do not

- Trigger the DAG. Backfills cost Snowflake compute; humans trigger them via the Airflow UI after reviewing.
- Hardcode credentials. All vendor credentials come from Airflow Connections, never from the DAG source.
- Skip the smoke test. A DAG that fails to parse is invisible in the Airflow UI until someone notices it's missing.
