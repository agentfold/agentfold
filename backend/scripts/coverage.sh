#!/usr/bin/env bash
# Runs the Go test suite and fails unless coverage is 100%.
# Excluded from the count: cmd/*/main.go bootstraps and generated code
# (any file whose header says "Code generated ... DO NOT EDIT.").
set -euo pipefail

cd "$(dirname "$0")/.."

out_dir="${COVERAGE_DIR:-coverage}"
mkdir -p "$out_dir"
raw="$out_dir/coverage.raw.out"
profile="$out_dir/coverage.out"

go test -race -covermode=atomic -coverpkg=./... -coverprofile="$raw" ./...

module="$(go list -m)"

# Collect excluded files as module-qualified paths.
excluded="$out_dir/excluded.txt"
{
  find cmd -name main.go 2>/dev/null || true
  grep -rlE --include='*.go' '^// Code generated .* DO NOT EDIT\.$' . 2>/dev/null | sed 's|^\./||' || true
} | sed "s|^|$module/|" | sort -u >"$excluded"

awk -v excluded="$excluded" '
  BEGIN { while ((getline line < excluded) > 0) skip[line] = 1 }
  NR == 1 { print; next }
  { split($0, parts, ":"); if (!(parts[1] in skip)) print }
' "$raw" >"$profile"

go tool cover -func="$profile" | tee "$out_dir/coverage.txt" | tail -1

total="$(go tool cover -func="$profile" | awk '/^total:/ { sub(/%/, "", $3); print $3 }')"
if [[ "$total" != "100.0" ]]; then
  echo "coverage is ${total}%, required 100%. Uncovered:" >&2
  go tool cover -func="$profile" | awk '$3 != "100.0%" && !/^total:/' >&2
  exit 1
fi
echo "coverage OK: 100%"
