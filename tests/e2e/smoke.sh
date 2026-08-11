#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:?set BASE_URL, e.g. http://45.55.96.251}"
HOST_HEADER="${HOST_HEADER:-staging.courtside}"

req() { curl -sS -m 10 -H "Host: ${HOST_HEADER}" "$@"; }
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; exit 1; }

echo "E2E smoke against ${BASE_URL} (Host: ${HOST_HEADER})"

club=$(req "${BASE_URL}/clubs/1")
echo "clubs/1 -> ${club}"
echo "${club}" | jq -e '.id == "1"' >/dev/null || fail "clubs/1 missing id"
echo "${club}" | jq -e '.members | length > 0' >/dev/null || fail "clubs/1 not enriched with members"
pass "sync enrichment (clubs -> members)"

req -X POST "${BASE_URL}/memberships" \
  -H 'Content-Type: application/json' \
  -d '{"member_id":"1","club_id":"1","plan":"premium"}' >/dev/null
pass "POST /memberships accepted"

ok=""
for i in $(seq 1 10); do
  if req "${BASE_URL}/invoices" | jq -e 'length > 0' >/dev/null 2>&1; then ok=yes; break; fi
  sleep 3
done
[ -n "${ok}" ] || fail "no invoice appeared (billing did not consume the event)"
pass "async billing reacted (invoice created)"

ok=""
for i in $(seq 1 10); do
  if req "${BASE_URL}/notifications" | jq -e 'length > 0' >/dev/null 2>&1; then ok=yes; break; fi
  sleep 3
done
[ -n "${ok}" ] || fail "no notification appeared (notifications did not consume the event)"
pass "async notifications reacted"

echo "ALL E2E SMOKE CHECKS PASSED"
