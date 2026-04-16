---
project: dataflow
framework: airflow
language: python
version: "3.12"
---

# Project Overview

## Architecture

Dataflow is an ETL pipeline platform built on Apache Airflow 2.9. It ingests data from vendor APIs, transforms it through pandas and dbt, and loads into Snowflake for analytics.

### Directory Structure

- `dags/` — Airflow DAG definitions, one file per pipeline
- `operators/` — Custom Airflow operators for vendor integrations
- `transforms/` — Pure transformation functions, no I/O
- `loaders/` — Snowflake and S3 write logic
- `schemas/` — Pydantic models for API responses and intermediate data
- `tests/` — Pytest tests, mirroring source structure

### Data Flow

1. Extractors pull raw JSON from vendor APIs using httpx with retry
2. Validators parse responses through Pydantic models, rejecting malformed records
3. Transformers apply business logic as pure functions on DataFrames
4. Loaders write parquet to S3 staging, then COPY INTO Snowflake

## Conventions

### Type Annotations

All functions must have type annotations. Use `typing.Protocol` for dependency injection boundaries. Avoid `Any` — prefer `object` or explicit union types.

### Error Handling

- Extractors retry transient HTTP errors (429, 502, 503) with exponential backoff
- Validators log rejected records to a dead letter queue, never raise on bad data
- Transformers raise `ValueError` on invariant violations — these are bugs, not data issues
- Loaders are idempotent: re-running a load for the same partition overwrites cleanly

### Testing

- Unit tests for transforms: pure input/output, no mocking needed
- Integration tests for extractors: use `respx` to mock HTTP, never hit real APIs
- DAG validation tests: verify all DAGs parse without import errors
- No snapshot tests — they broke on every pandas version bump
