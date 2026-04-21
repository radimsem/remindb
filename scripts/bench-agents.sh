#!/usr/bin/env bash
# Run `remindb bench` against every agent testdata vault and print one table per agent.
# Each agent's vault is compiled into an isolated DB under a temp dir that is cleaned up on exit.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TESTDATA="$REPO_ROOT/testdata"

BUDGET=1000

AGENTS=(openclaw claude-code codex gemini-cli)

# Per-agent search queries.
declare -A QUERIES=(
  [openclaw]="WebSocket persistent connection rate limit|Sentry alert threshold deploy window|stale memory flagged review"
  [claude-code]="Stripe webhook idempotency key cart|PostgreSQL connection pool PgBouncer|drizzle migration NOT NULL backfill"
  [codex]="WebSocket operator Vendor C blocked|dead letter queue rejected records|Snowflake COPY INTO parquet"
  [gemini-cli]="exponential backoff jitter DefaultBackoff|PLAT 1903 retry storm ConfigMap reconciler|Vault token renewer silent alerting"
)

WORK_DIR="$(mktemp -d -t remindb-bench-agents-XXXXXX)"
trap 'rm -rf "$WORK_DIR"' EXIT

cd "$REPO_ROOT"

echo "Building remindb..."
go build -o "$WORK_DIR/remindb" ./cmd/remindb

for agent in "${AGENTS[@]}"; do
  src="$TESTDATA/$agent"
  db="$WORK_DIR/$agent.db"

  if [[ ! -d "$src" ]]; then
    echo "skip: $agent (no such directory: $src)" >&2
    continue
  fi

  echo
  echo "=== $agent ==="

  "$WORK_DIR/remindb" compile "$src" --db "$db" >/dev/null

  # Split this agent's pipe-delimited query list into repeated --query flags.
  query_args=()
  IFS='|' read -ra agent_queries <<<"${QUERIES[$agent]}"
  for q in "${agent_queries[@]}"; do
    query_args+=(--query "$q")
  done

  "$WORK_DIR/remindb" bench --db "$db" --dir "$src" --budget "$BUDGET" "${query_args[@]}"
done
