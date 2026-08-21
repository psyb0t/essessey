#!/usr/bin/env bash
# Security scan: govulncheck (reachable Go vulnerabilities) + semgrep (SAST),
# merged into one SARIF for upload to the Security tab, and gating: a finding
# from either tool exits non-zero. Vendored third-party code is out of scope for
# semgrep. Needs govulncheck, semgrep, jq (all in Dockerfile.dev).
set -euo pipefail

SARIF_OUT="${SARIF_OUT:-sec.sarif}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "== govulncheck =="
# SARIF mode exits 0 even with findings (report only); the text run below gates,
# exiting non-zero on a reachable vulnerability.
govulncheck -format sarif ./... >"$tmp/govulncheck.sarif"
gv_rc=0
govulncheck ./... || gv_rc=$?

echo "== semgrep (p/golang + p/security-audit) =="
sg_rc=0
semgrep scan \
	--config=p/golang \
	--config=p/security-audit \
	--exclude=vendor \
	--sarif --output="$tmp/semgrep.sarif" \
	--metrics=off \
	--error \
	. || sg_rc=$?

echo "== merging SARIF into ${SARIF_OUT} =="
jq -s '{version:"2.1.0","$schema":"https://json.schemastore.org/sarif-2.1.0.json",runs:((.[0].runs // []) + (.[1].runs // []))}' \
	"$tmp/govulncheck.sarif" "$tmp/semgrep.sarif" >"$SARIF_OUT"

if [ "$gv_rc" -ne 0 ] || [ "$sg_rc" -ne 0 ]; then
	echo "FAIL: security findings (govulncheck=${gv_rc}, semgrep=${sg_rc}). See ${SARIF_OUT}"
	exit 1
fi

echo "No security findings."
