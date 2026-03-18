#!/bin/bash
# demo-all-scenarios.sh — Exercise all vendor stub scenarios + over-limit rule.
# Runs against a running docker compose stack.
# Usage: ./scripts/demo-all-scenarios.sh
#
# Vendor stub scenarios are triggered by passing vendor_scenario= to the deposit
# endpoint. Account IDs are chosen to match the scenario by convention; the stub
# routes responses based on the explicit scenario field.

set -euo pipefail

BASE="http://localhost:8080"
TOKEN="tok_investor_test"
OP="OP-001"
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

# submit_deposit <account_id> <amount_cents> [vendor_scenario]
submit_deposit() {
    local account_id="$1" amount_cents="$2" scenario="${3:-}"
    local extra_fields=""
    if [ -n "$scenario" ]; then
        extra_fields="-F vendor_scenario=$scenario"
    fi
    # shellcheck disable=SC2086
    curl -s -X POST "$BASE/api/v1/deposits" \
        -H "Authorization: Bearer $TOKEN" \
        -F "account_id=$account_id" \
        -F "amount_cents=$amount_cents" \
        -F "front_image=@$FRONT" \
        -F "back_image=@$BACK" \
        $extra_fields
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
AMOUNT_1006=$((10000 + RANDOM % 9000))   # 10000–18999
AMOUNT_0000=$((20000 + RANDOM % 9000))   # 20000–28999
AMOUNT_RETIRE=$((30000 + RANDOM % 9000)) # 30000–38999

# ── Scenario 1: IQA Blur ──────────────────────────────────────────────────────
echo "Scenario 1: IQA_FAIL_BLUR — image too blurry (expect: rejected)"
RESP=$(submit_deposit "ACC-SOFI-1001" "50000" "IQA_FAIL_BLUR")
STATUS=$(echo "$RESP" | jq -r '.data.status')
REJECTION_REASON=$(echo "$RESP" | jq -r '.data.rejection_reason // empty')
assert_eq "IQA blur → rejected" "$STATUS" "rejected"
if [ -n "$REJECTION_REASON" ]; then
    pass "rejection_reason is set: $REJECTION_REASON"
else
    fail "rejection_reason should be set on rejected transfer"
fi
echo ""

# ── Scenario 2: IQA Glare ─────────────────────────────────────────────────────
echo "Scenario 2: IQA_FAIL_GLARE — image has glare (expect: rejected)"
RESP=$(submit_deposit "ACC-SOFI-1002" "50000" "IQA_FAIL_GLARE")
STATUS=$(echo "$RESP" | jq -r '.data.status')
assert_eq "IQA glare → rejected" "$STATUS" "rejected"
echo ""

# ── Scenario 3: MICR Read Failure ─────────────────────────────────────────────
echo "Scenario 3: MICR_READ_FAILURE — MICR line unreadable (expect: analyzing + flagged)"
RESP=$(submit_deposit "ACC-SOFI-1003" "75000" "MICR_READ_FAILURE")
STATUS=$(echo "$RESP" | jq -r '.data.status')
FLAGGED=$(echo "$RESP" | jq -r '.data.flagged')
FLAG_REASON=$(echo "$RESP" | jq -r '.data.flag_reason')
assert_eq "MICR failure → analyzing" "$STATUS" "analyzing"
assert_eq "MICR failure is flagged=true" "$FLAGGED" "true"
assert_eq "flag_reason is micr_failure" "$FLAG_REASON" "micr_failure"
echo ""

# ── Scenario 4: Duplicate Detected ────────────────────────────────────────────
echo "Scenario 4: DUPLICATE_DETECTED — check already deposited (expect: rejected)"
RESP=$(submit_deposit "ACC-SOFI-1004" "60000" "DUPLICATE_DETECTED")
STATUS=$(echo "$RESP" | jq -r '.data.status')
assert_eq "duplicate detected → rejected" "$STATUS" "rejected"
echo ""

# ── Scenario 5: Amount Mismatch ───────────────────────────────────────────────
echo "Scenario 5: AMOUNT_MISMATCH — OCR differs from declared (expect: analyzing + flagged)"
RESP=$(submit_deposit "ACC-SOFI-1005" "80000" "AMOUNT_MISMATCH")
STATUS=$(echo "$RESP" | jq -r '.data.status')
FLAGGED=$(echo "$RESP" | jq -r '.data.flagged')
FLAG_REASON=$(echo "$RESP" | jq -r '.data.flag_reason')
FLAGGED_TRANSFER_ID=$(echo "$RESP" | jq -r '.data.transfer_id')
assert_eq "amount mismatch → analyzing" "$STATUS" "analyzing"
assert_eq "amount mismatch is flagged=true" "$FLAGGED" "true"
assert_eq "flag_reason is amount_mismatch" "$FLAG_REASON" "amount_mismatch"
echo ""

# ── Scenario 6: Clean Pass ────────────────────────────────────────────────────
echo "Scenario 6: CLEAN_PASS — all checks pass (expect: funds_posted) [amount: ${AMOUNT_1006}]"
RESP=$(submit_deposit "ACC-SOFI-1006" "$AMOUNT_1006")
STATUS=$(echo "$RESP" | jq -r '.data.status')
assert_eq "clean pass → funds_posted" "$STATUS" "funds_posted"
echo ""

# ── Scenario 7: Basic Pass ────────────────────────────────────────────────────
echo "Scenario 7: CLEAN_PASS default — basic pass (expect: funds_posted) [amount: ${AMOUNT_0000}]"
RESP=$(submit_deposit "ACC-SOFI-0000" "$AMOUNT_0000")
STATUS=$(echo "$RESP" | jq -r '.data.status')
assert_eq "basic pass → funds_posted" "$STATUS" "funds_posted"
echo ""

# ── Scenario 8: Over-Limit ($6,000 > $5,000 max) ─────────────────────────────
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

# ── Scenario 9: Retirement Account — Contribution Type ───────────────────────
echo "Scenario 9: Retirement account — contribution_type should default to INDIVIDUAL"
RESP=$(submit_deposit "ACC-RETIRE-001" "$AMOUNT_RETIRE")
STATUS=$(echo "$RESP" | jq -r '.data.status')
CT=$(echo "$RESP" | jq -r '.data.contribution_type // empty')
assert_eq "retirement account → funds_posted" "$STATUS" "funds_posted"
if [ "$CT" = "INDIVIDUAL" ]; then
    pass "contribution_type=INDIVIDUAL for retirement account"
else
    fail "contribution_type should be INDIVIDUAL, got: '$CT'"
fi
echo ""

# ── Scenario 10: Operator Review — Approve Flagged Deposit ───────────────────
echo "Scenario 10: Operator review — approve a flagged (MICR failure) deposit"
MICR_RESP=$(submit_deposit "ACC-SOFI-1003" "90000" "MICR_READ_FAILURE")
MICR_ID=$(echo "$MICR_RESP" | jq -r '.data.transfer_id')
MICR_STATUS=$(echo "$MICR_RESP" | jq -r '.data.status')
assert_eq "flagged deposit in reviewing queue (analyzing)" "$MICR_STATUS" "analyzing"

# Verify it appears in operator queue
QUEUE=$(curl -s "$BASE/api/v1/operator/queue" -H "X-Operator-ID: $OP")
QUEUE_COUNT=$(echo "$QUEUE" | jq '[.data[] | select(.transfer_id == "'"$MICR_ID"'")] | length')
assert_eq "flagged deposit appears in operator queue" "$QUEUE_COUNT" "1"

# Approve the deposit
APPROVE_RESP=$(curl -s -X POST "$BASE/api/v1/operator/deposits/$MICR_ID/approve" \
    -H "X-Operator-ID: $OP" \
    -H "Content-Type: application/json" \
    -d '{"notes":"MICR verified manually, data confirmed"}')
APPROVE_STATUS=$(echo "$APPROVE_RESP" | jq -r '.data.status')
assert_eq "operator approve → funds_posted" "$APPROVE_STATUS" "funds_posted"
echo ""

# ── Scenario 11: Operator Review — Reject Flagged Deposit + Audit Log ─────────
echo "Scenario 11: Operator review — reject a flagged deposit and verify audit log"
REJECT_RESP=$(submit_deposit "ACC-SOFI-1003" "95000" "MICR_READ_FAILURE")
REJECT_ID=$(echo "$REJECT_RESP" | jq -r '.data.transfer_id')
REJECT_STATUS=$(echo "$REJECT_RESP" | jq -r '.data.status')
assert_eq "second flagged deposit in queue" "$REJECT_STATUS" "analyzing"

# Reject the deposit
REJECT_ACTION=$(curl -s -X POST "$BASE/api/v1/operator/deposits/$REJECT_ID/reject" \
    -H "X-Operator-ID: $OP" \
    -H "Content-Type: application/json" \
    -d '{"reason":"MICR ink too light or faded","notes":"Unable to verify routing number"}')
REJECT_FINAL=$(echo "$REJECT_ACTION" | jq -r '.data.status')
assert_eq "operator reject → rejected" "$REJECT_FINAL" "rejected"

# Verify audit log entry
AUDIT=$(curl -s "$BASE/api/v1/operator/audit" -H "X-Operator-ID: $OP")
AUDIT_COUNT=$(echo "$AUDIT" | jq '[.data[] | select(.transfer_id == "'"$REJECT_ID"'" and .action == "reject")] | length')
assert_eq "audit log has reject entry for transfer" "$AUDIT_COUNT" "1"
echo ""

# ── Summary ───────────────────────────────────────────────────────────────────
TOTAL=$((PASS+FAIL))
echo "=============================="
echo " Results: $PASS/$TOTAL tests passed"
echo "=============================="

if [ "$FAIL" -gt "0" ]; then
    exit 1
fi
