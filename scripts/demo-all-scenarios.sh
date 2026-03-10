#!/bin/bash
# demo-all-scenarios.sh — Exercise all 7 vendor stub scenarios + over-limit rule.
# Runs against a running docker compose stack.
# Usage: ./scripts/demo-all-scenarios.sh

set -euo pipefail

BASE="http://localhost:8080"
TOKEN="tok_investor_test"
FRONT="scripts/fixtures/check-front.png"
BACK="scripts/fixtures/check-back.png"

PASS=0
FAIL=0

pass() { echo "  ✓ PASS: $1"; PASS=$((PASS+1)); }
fail() { echo "  ✗ FAIL: $1"; FAIL=$((FAIL+1)); }

assert_eq() {
    local label="$1" got="$2" want="$3"
    if [ "$got" = "$want" ]; then
        pass "$label (got: $got)"
    else
        fail "$label (want: $want, got: $got)"
    fi
}

submit_deposit() {
    local account_id="$1" amount_cents="$2"
    curl -s -X POST "$BASE/api/v1/deposits" \
        -H "Authorization: Bearer $TOKEN" \
        -F "account_id=$account_id" \
        -F "amount_cents=$amount_cents" \
        -F "front_image=@$FRONT" \
        -F "back_image=@$BACK"
}

echo "=============================="
echo " DEMO: All Vendor Stub Scenarios"
echo " $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
echo "=============================="
echo ""

# ── Setup: Flush Redis to avoid stale duplicate hashes ───────────────────────
echo "Setup: Flushing Redis (clears stale duplicate-check hashes)"
FLUSH_OK=0
if DOCKER_HOST=unix:///var/run/docker.sock docker exec apex-mobile-check-deposit-redis-1 redis-cli FLUSHALL >/dev/null 2>&1; then
    echo "  Redis flushed (DOCKER_HOST=unix:///var/run/docker.sock)"
    FLUSH_OK=1
elif docker exec apex-mobile-check-deposit-redis-1 redis-cli FLUSHALL >/dev/null 2>&1; then
    echo "  Redis flushed (default docker)"
    FLUSH_OK=1
fi
if [ "$FLUSH_OK" -eq 0 ]; then
    echo "  WARNING: Redis flush failed — stale hashes may cause false duplicate rejections"
fi
echo ""

# Use non-overlapping random ranges to avoid duplicate hash collisions.
# Vendor stub always returns routing=021000021, account=123456789, serial=0001.
AMOUNT_1006=$((10000 + RANDOM % 9000))   # 10000–18999
AMOUNT_0000=$((20000 + RANDOM % 9000))   # 20000–28999

# ── Scenario 1: ACC-SOFI-1001 — IQA Blur ─────────────────────────────────────
echo "Scenario 1: ACC-SOFI-1001 — IQA Blur (expect: rejected)"
RESP=$(submit_deposit "ACC-SOFI-1001" "50000")
STATUS=$(echo "$RESP" | jq -r '.data.status')
assert_eq "IQA blur account is rejected" "$STATUS" "rejected"
echo ""

# ── Scenario 2: ACC-SOFI-1002 — IQA Glare ────────────────────────────────────
echo "Scenario 2: ACC-SOFI-1002 — IQA Glare (expect: rejected)"
RESP=$(submit_deposit "ACC-SOFI-1002" "50000")
STATUS=$(echo "$RESP" | jq -r '.data.status')
assert_eq "IQA glare account is rejected" "$STATUS" "rejected"
echo ""

# ── Scenario 3: ACC-SOFI-1003 — MICR Read Failure ────────────────────────────
echo "Scenario 3: ACC-SOFI-1003 — MICR Read Failure (expect: analyzing + flagged + micr_failure)"
RESP=$(submit_deposit "ACC-SOFI-1003" "75000")
STATUS=$(echo "$RESP" | jq -r '.data.status')
FLAGGED=$(echo "$RESP" | jq -r '.data.flagged')
FLAG_REASON=$(echo "$RESP" | jq -r '.data.flag_reason')
assert_eq "MICR failure reaches analyzing" "$STATUS" "analyzing"
assert_eq "MICR failure is flagged=true" "$FLAGGED" "true"
assert_eq "flag_reason is micr_failure" "$FLAG_REASON" "micr_failure"
echo ""

# ── Scenario 4: ACC-SOFI-1004 — Duplicate Detected ───────────────────────────
echo "Scenario 4: ACC-SOFI-1004 — Duplicate Detected (expect: rejected)"
RESP=$(submit_deposit "ACC-SOFI-1004" "60000")
STATUS=$(echo "$RESP" | jq -r '.data.status')
assert_eq "duplicate account is rejected" "$STATUS" "rejected"
echo ""

# ── Scenario 5: ACC-SOFI-1005 — Amount Mismatch ──────────────────────────────
echo "Scenario 5: ACC-SOFI-1005 — Amount Mismatch (expect: analyzing + flagged + amount_mismatch)"
RESP=$(submit_deposit "ACC-SOFI-1005" "80000")
STATUS=$(echo "$RESP" | jq -r '.data.status')
FLAGGED=$(echo "$RESP" | jq -r '.data.flagged')
FLAG_REASON=$(echo "$RESP" | jq -r '.data.flag_reason')
assert_eq "amount mismatch reaches analyzing" "$STATUS" "analyzing"
assert_eq "amount mismatch is flagged=true" "$FLAGGED" "true"
assert_eq "flag_reason is amount_mismatch" "$FLAG_REASON" "amount_mismatch"
echo ""

# ── Scenario 6: ACC-SOFI-1006 — Clean Pass ───────────────────────────────────
echo "Scenario 6: ACC-SOFI-1006 — Clean Pass (expect: funds_posted) [amount: ${AMOUNT_1006}]"
RESP=$(submit_deposit "ACC-SOFI-1006" "$AMOUNT_1006")
STATUS=$(echo "$RESP" | jq -r '.data.status')
assert_eq "clean pass reaches funds_posted" "$STATUS" "funds_posted"
echo ""

# ── Scenario 7: ACC-SOFI-0000 — Basic Pass ───────────────────────────────────
echo "Scenario 7: ACC-SOFI-0000 — Basic Pass (expect: funds_posted) [amount: ${AMOUNT_0000}]"
RESP=$(submit_deposit "ACC-SOFI-0000" "$AMOUNT_0000")
STATUS=$(echo "$RESP" | jq -r '.data.status')
assert_eq "basic pass reaches funds_posted" "$STATUS" "funds_posted"
echo ""

# ── Scenario 8: Over-Limit (600000 cents = $6,000) ────────────────────────────
echo "Scenario 8: Over-limit deposit (600000 cents — expect: 422 DEPOSIT_OVER_LIMIT)"
RESP=$(curl -s -w '\n%{http_code}' -X POST "$BASE/api/v1/deposits" \
    -H "Authorization: Bearer $TOKEN" \
    -F "account_id=ACC-SOFI-0000" \
    -F "amount_cents=600000" \
    -F "front_image=@$FRONT" \
    -F "back_image=@$BACK")
BODY=$(echo "$RESP" | head -n1)
HTTP_CODE=$(echo "$RESP" | tail -n1)
CODE_FIELD=$(echo "$BODY" | jq -r '.code')
assert_eq "over-limit returns HTTP 422" "$HTTP_CODE" "422"
assert_eq "over-limit error code is DEPOSIT_OVER_LIMIT" "$CODE_FIELD" "DEPOSIT_OVER_LIMIT"
echo ""

# ── Summary ───────────────────────────────────────────────────────────────────
TOTAL=$((PASS+FAIL))
echo "=============================="
echo " Results: $PASS/$TOTAL tests passed"
echo "=============================="

if [ "$FAIL" -gt "0" ]; then
    exit 1
fi
