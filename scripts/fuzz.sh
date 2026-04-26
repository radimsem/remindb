#!/usr/bin/env bash
# Run every Fuzz* target in the repo for $FUZZTIME each (default 30s).
# Discovers targets via `go test -list`; one `go test -fuzz=` invocation per
# target because the flag accepts only one matching target per package.
#
# Usage:
#   scripts/fuzz.sh           # 30s per target
#   scripts/fuzz.sh 1m        # 1m per target
#   FUZZTIME=2m scripts/fuzz.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

FUZZTIME="${FUZZTIME:-${1:-30s}}"

cd "$REPO_ROOT"

count=0
start=$SECONDS

for pkg in $(go list ./...); do
  targets=$(go test -list='^Fuzz' "$pkg" 2>/dev/null | grep '^Fuzz' || true)
  if [[ -z "$targets" ]]; then
    continue
  fi

  for t in $targets; do
    echo
    echo "=== $pkg :: $t (fuzztime=$FUZZTIME) ==="
    # -run='^$' suppresses normal Test* runs so CPU stays on the fuzzer.
    go test -run='^$' -fuzz="^${t}\$" -fuzztime="$FUZZTIME" "$pkg"
    count=$((count + 1))
  done
done

elapsed=$((SECONDS - start))
echo
echo "fuzzed $count targets in ${elapsed}s (no crashes)"
