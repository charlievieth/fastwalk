#!/usr/bin/env bash

set -euo pipefail

COUNT=5
TESTS=(
    'filepath'
    'godirwalk'
    'fastwalk'
)

ROOT="$(go env GOROOT)"
if [[ ! -d "${ROOT}" ]]; then
    echo >&2 "error: GOROOT (\"${ROOT}\") does not exist and is required to run benchmarks"
    exit 1
fi

TEST_FLAGS=(
    -run '^$' # skip all tests
    -bench '^BenchmarkWalkComparison$'
    -benchmem
    -count "${COUNT}"
)
TMP="$(mktemp -d -t fastwalk-bench.XXXXXX)"

for name in "${TESTS[@]}"; do
    echo "## ${name}"
    go test "${TEST_FLAGS[@]}" github.com/charlievieth/fastwalk -walkfunc "${name}" |
        tee "${TMP}/${name}.out"
    echo ''
done

echo '## Comparisons'
echo '########################################################'
echo ''

echo '## filepath vs. fastwalk'
benchstat "${TMP}/filepath.out" "${TMP}/fastwalk.out"
echo ''

echo '## godirwalk vs. fastwalk'
benchstat "${TMP}/godirwalk.out" "${TMP}/fastwalk.out"
echo ''

echo "## Temp: ${TMP}"
