#!/usr/bin/env bash
# Race-enabled coverage gate. Busybox-safe: awk, not grep -P, so it runs in the
# alpine dev image. Writes coverage-percent.txt for the badges workflow and fails
# if total coverage is below MIN_TEST_COVERAGE.
set -euo pipefail

MIN_TEST_COVERAGE="${MIN_TEST_COVERAGE:-90}"
trap 'rm -f coverage.txt' EXIT

go test -race -coverpkg=./... -coverprofile=coverage.txt ./...

pct="$(go tool cover -func=coverage.txt | awk '/^total:/ { gsub(/%/, "", $3); print $3 }')"
: "${pct:=0}"
printf '%s\n' "$pct" >coverage-percent.txt

result="${pct%.*}"
: "${result:=0}"

if [ "$result" -eq 0 ]; then
	echo "No test coverage information available."
	exit 0
fi

if [ "$result" -lt "$MIN_TEST_COVERAGE" ]; then
	echo "FAIL: Coverage ${pct}% is less than the minimum ${MIN_TEST_COVERAGE}%"
	exit 1
fi

echo "Coverage ${pct}% meets the minimum ${MIN_TEST_COVERAGE}%"
