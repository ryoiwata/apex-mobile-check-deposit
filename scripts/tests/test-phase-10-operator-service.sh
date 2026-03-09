#!/bin/bash
# Phase 10 acceptance tests — operator service: review queue, approve/reject, audit logging.
# Runs against an already-running Docker Compose stack.
# Usage: ./scripts/tests/test-phase10-operator-service.sh

set -euo pipefail

BASE="http://localhost:8080"
INVESTOR_TOKEN="tok_investor_test"
OPERATOR_ID="OP-001"
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

assert_gte() {
    local label="$1" got="$2" want="$3"
    if [ "$got" -ge "$want" ]; then
        pass "$label (got: $got, want >= $want)"
    else
        fail "$label (want >= $want, got: $got)"
    fi
}

submit_deposit() {
    local account_id="$1" amount_cents="$2"
    curl -s -X POST "$BASE/api/v1/deposits" \
        -H "Authorization: Bearer $INVESTOR_TOKEN" \
        -F "account_id=$account_id" \
        -F "amount_cents=$amount_cents" \
        -F "front_image=@$FRONT" \
        -F "back_image=@$BACK"
}

operator_get() {
    local path="$1"
    curl -s "$BASE/api/v1/operator/$path" \
        -H "X-Operator-ID: $OPERATOR_ID"
}

operator_post() {
    local path="$1" body="$2"
    curl -s -X POST "$BASE/api/v1/operator/$path" \
        -H "X-Operator-ID: $OPERATOR_ID" \
        -H "Content-Type: application/json" \
        -d "$body"
}

echo "=============================="
echo " Phase 10 Acceptance Tests"
echo " $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
echo "=============================="
echo ""

# ── Test a: Submit MICR-failure deposit — assert flagged + analyzing ──────────
echo "a. MICR-failure deposit (ACC-SOFI-1003) — should be flagged and analyzing"
MICR_AMOUNT=$((10000 + RANDOM % 90000))
RESP=$(submit_deposit "ACC-SOFI-1003" "$MICR_AMOUNT")
MICR_STATUS=$(echo "$RESP" | jq -r '.data.status')
MICR_FLAGGED=$(echo "$RESP" | jq -r '.data.flagged')
MICR_ID=$(echo "$RESP" | jq -r '.data.transfer_id')
assert_eq "MICR failure reaches analyzing" "$MICR_STATUS" "analyzing"
assert_eq "MICR failure is flagged" "$MICR_FLAGGED" "true"
echo ""

# ── Test b: Submit amount-mismatch deposit — assert flagged + analyzing ───────
echo "b. Amount-mismatch deposit (ACC-SOFI-1005) — should be flagged and analyzing"
MISMATCH_AMOUNT=$((10000 + RANDOM % 90000))
RESP=$(submit_deposit "ACC-SOFI-1005" "$MISMATCH_AMOUNT")
MISMATCH_STATUS=$(echo "$RESP" | jq -r '.data.status')
MISMATCH_FLAGGED=$(echo "$RESP" | jq -r '.data.flagged')
MISMATCH_ID=$(echo "$RESP" | jq -r '.data.transfer_id')
assert_eq "amount mismatch reaches analyzing" "$MISMATCH_STATUS" "analyzing"
assert_eq "amount mismatch is flagged" "$MISMATCH_FLAGGED" "true"
echo ""

# ── Test c: GET /operator/queue — queue contains at least 2 flagged deposits ──
echo "c. GET /operator/queue — queue length >= 2"
QUEUE=$(operator_get "queue")
QUEUE_LEN=$(echo "$QUEUE" | jq '.data | length')
assert_gte "queue contains at least 2 flagged deposits" "$QUEUE_LEN" "2"
echo ""

# ── Test d: Approve MICR-failure deposit — assert funds_posted ────────────────
echo "d. Approve MICR-failure deposit ($MICR_ID)"
APPROVE_RESP=$(operator_post "deposits/$MICR_ID/approve" \
    '{"notes":"MICR verified manually, routing confirmed"}')
APPROVE_STATUS=$(echo "$APPROVE_RESP" | jq -r '.data.status')
assert_eq "approved deposit reaches funds_posted" "$APPROVE_STATUS" "funds_posted"
echo ""

# ── Test e: GET /operator/queue — queue length decreased by 1 ─────────────────
echo "e. GET /operator/queue after approve — length decreased"
QUEUE_AFTER=$(operator_get "queue")
QUEUE_LEN_AFTER=$(echo "$QUEUE_AFTER" | jq '.data | length')
EXPECTED_LEN=$((QUEUE_LEN - 1))
assert_eq "queue length decreased by 1 after approve" "$QUEUE_LEN_AFTER" "$EXPECTED_LEN"
echo ""

# ── Test f: Audit log for approved deposit — action=approve, operator=OP-001 ──
echo "f. Audit log for approved deposit — action=approve, operator_id=OP-001"
AUDIT=$(operator_get "audit?transfer_id=$MICR_ID")
AUDIT_COUNT=$(echo "$AUDIT" | jq '.data | length')
AUDIT_ACTION=$(echo "$AUDIT" | jq -r '.data[0].action')
AUDIT_OPERATOR=$(echo "$AUDIT" | jq -r '.data[0].operator_id')
if [ "$AUDIT_COUNT" -gt "0" ]; then
    pass "audit log has entries for approved deposit (count: $AUDIT_COUNT)"
else
    fail "audit log is empty for approved deposit"
fi
assert_eq "audit action is approve" "$AUDIT_ACTION" "approve"
assert_eq "audit operator_id is OP-001" "$AUDIT_OPERATOR" "OP-001"
echo ""

# ── Test g: Reject amount-mismatch deposit — assert rejected ──────────────────
echo "g. Reject amount-mismatch deposit ($MISMATCH_ID)"
REJECT_RESP=$(operator_post "deposits/$MISMATCH_ID/reject" \
    '{"reason":"Entered amount does not match OCR reading","notes":"Discrepancy too large"}')
REJECT_STATUS=$(echo "$REJECT_RESP" | jq -r '.data.status')
assert_eq "rejected deposit is now rejected" "$REJECT_STATUS" "rejected"
echo ""

# ── Test h: Audit log for rejected deposit — action=reject, operator=OP-001 ───
echo "h. Audit log for rejected deposit — action=reject, operator_id=OP-001"
REJECT_AUDIT=$(operator_get "audit?transfer_id=$MISMATCH_ID")
REJECT_AUDIT_COUNT=$(echo "$REJECT_AUDIT" | jq '.data | length')
REJECT_AUDIT_ACTION=$(echo "$REJECT_AUDIT" | jq -r '.data[0].action')
REJECT_AUDIT_OPERATOR=$(echo "$REJECT_AUDIT" | jq -r '.data[0].operator_id')
if [ "$REJECT_AUDIT_COUNT" -gt "0" ]; then
    pass "audit log has entries for rejected deposit (count: $REJECT_AUDIT_COUNT)"
else
    fail "audit log is empty for rejected deposit"
fi
assert_eq "audit action is reject" "$REJECT_AUDIT_ACTION" "reject"
assert_eq "audit operator_id is OP-001" "$REJECT_AUDIT_OPERATOR" "OP-001"
echo ""

# ── Test i: GET /operator/queue — both deposits removed, length decreased ──────
echo "i. GET /operator/queue after reject — length decreased again"
QUEUE_FINAL=$(operator_get "queue")
QUEUE_LEN_FINAL=$(echo "$QUEUE_FINAL" | jq '.data | length')
# Both deposits have been handled; length should be 2 less than original
EXPECTED_FINAL=$((QUEUE_LEN - 2))
if [ "$EXPECTED_FINAL" -lt "0" ]; then EXPECTED_FINAL=0; fi
assert_eq "queue length decreased after reject" "$QUEUE_LEN_FINAL" "$EXPECTED_FINAL"
echo ""

# ── Test j: Approve non-flagged deposit — expect 409 ─────────────────────────
echo "j. Approve non-flagged deposit — expect 409 conflict"
CLEAN_AMOUNT=$((5000 + RANDOM % 5000))
CLEAN_RESP=$(submit_deposit "ACC-SOFI-1006" "$CLEAN_AMOUNT")
CLEAN_ID=$(echo "$CLEAN_RESP" | jq -r '.data.transfer_id')
CLEAN_STATUS=$(echo "$CLEAN_RESP" | jq -r '.data.status')
# This deposit goes straight to funds_posted — not in operator queue
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "$BASE/api/v1/operator/deposits/$CLEAN_ID/approve" \
    -H "X-Operator-ID: $OPERATOR_ID" \
    -H "Content-Type: application/json" \
    -d '{"notes":"should fail"}')
assert_eq "clean deposit pre-approved → clean pass status is funds_posted" "$CLEAN_STATUS" "funds_posted"
assert_eq "approving non-flagged deposit returns 409" "$HTTP_CODE" "409"
echo ""

# ── Test k: Missing operator header returns 401 ───────────────────────────────
echo "k. GET /operator/queue without X-Operator-ID returns 401"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/api/v1/operator/queue")
assert_eq "missing operator header returns 401" "$HTTP_CODE" "401"
echo ""

# ── Test l: Approve with contribution_type_override — verify on transfer ──────
echo "l. Approve flagged deposit with contribution_type_override=EMPLOYER"
OVERRIDE_AMOUNT=$((20000 + RANDOM % 80000))
OVERRIDE_RESP=$(submit_deposit "ACC-SOFI-1003" "$OVERRIDE_AMOUNT")
OVERRIDE_ID=$(echo "$OVERRIDE_RESP" | jq -r '.data.transfer_id')
OVERRIDE_SUBMIT_STATUS=$(echo "$OVERRIDE_RESP" | jq -r '.data.status')
assert_eq "MICR deposit for override test is flagged/analyzing" "$OVERRIDE_SUBMIT_STATUS" "analyzing"

OVERRIDE_APPROVE=$(operator_post "deposits/$OVERRIDE_ID/approve" \
    '{"notes":"Override contribution type for employer rollover","contribution_type_override":"EMPLOYER"}')
OVERRIDE_APPROVE_STATUS=$(echo "$OVERRIDE_APPROVE" | jq -r '.data.status')
assert_eq "override-approved deposit reaches funds_posted" "$OVERRIDE_APPROVE_STATUS" "funds_posted"

# Check contribution_type on the transfer via deposit endpoint
TRANSFER_DETAIL=$(curl -s "$BASE/api/v1/deposits/$OVERRIDE_ID" \
    -H "Authorization: Bearer $INVESTOR_TOKEN")
CONTRIBUTION_TYPE=$(echo "$TRANSFER_DETAIL" | jq -r '.data.contribution_type')
assert_eq "contribution_type_override applied to transfer" "$CONTRIBUTION_TYPE" "EMPLOYER"
echo ""

# ── Summary ───────────────────────────────────────────────────────────────────
TOTAL=$((PASS+FAIL))
echo "=============================="
echo " Results: $PASS/$TOTAL tests passed"
echo "=============================="

if [ "$FAIL" -gt "0" ]; then
    exit 1
fi
