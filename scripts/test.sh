#!/usr/bin/env bash
# Full test sweep: unit + integration tests (including fuzz seed corpora),
# then fuzz every target for $FUZZTIME (default 30s).
#
# Use `make test` (`go test ./...`) for the fast inner-loop variant.
#
# Usage:
#   scripts/test.sh           # 30s per fuzz target
#   scripts/test.sh 1m        # 1m per fuzz target
#   FUZZTIME=2m scripts/test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$REPO_ROOT"

echo "=== go test ./... ==="
go test ./...

echo
echo "=== fuzz ==="
"$SCRIPT_DIR/fuzz.sh" "$@"
