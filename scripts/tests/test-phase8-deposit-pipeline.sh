#!/bin/bash
# Phase 8 acceptance tests — deposit pipeline, middleware, server wiring.
# Runs against an already-running Docker Compose stack.
# Usage: ./scripts/test-phase8-deposit-pipeline.sh

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
echo " Phase 8 Acceptance Tests"
echo " $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
echo "=============================="
echo ""

# ── Test a: Health check ─────────────────────────────────────────────────────
echo "a. Health check"
STATUS=$(curl -s "$BASE/health" | jq -r '.status')
assert_eq "health endpoint returns ok" "$STATUS" "ok"
echo ""

# ── Test b: Happy path — clean pass ──────────────────────────────────────────
# Use a random amount (1000–9999 cents) to avoid Redis duplicate detection
# across repeated test runs. The duplicate hash is keyed on routing+account+amount+serial.
echo "b. Happy path — clean pass (ACC-SOFI-1006)"
HAPPY_AMOUNT=$((1000 + RANDOM % 9000))
RESP=$(submit_deposit "ACC-SOFI-1006" "$HAPPY_AMOUNT")
STATUS=$(echo "$RESP" | jq -r '.data.status')
HAPPY_ID=$(echo "$RESP" | jq -r '.data.transfer_id')
assert_eq "clean pass reaches funds_posted" "$STATUS" "funds_posted"
echo ""

# ── Test c: IQA blur rejection ────────────────────────────────────────────────
echo "c. IQA blur rejection (ACC-SOFI-1001)"
RESP=$(submit_deposit "ACC-SOFI-1001" "50000")
STATUS=$(echo "$RESP" | jq -r '.data.status')
assert_eq "blur account is rejected" "$STATUS" "rejected"
echo ""

# ── Test d: IQA glare rejection ───────────────────────────────────────────────
echo "d. IQA glare rejection (ACC-SOFI-1002)"
RESP=$(submit_deposit "ACC-SOFI-1002" "50000")
STATUS=$(echo "$RESP" | jq -r '.data.status')
assert_eq "glare account is rejected" "$STATUS" "rejected"
echo ""

# ── Test e: MICR failure — flagged ────────────────────────────────────────────
echo "e. MICR failure flagged (ACC-SOFI-1003)"
RESP=$(submit_deposit "ACC-SOFI-1003" "75000")
STATUS=$(echo "$RESP" | jq -r '.data.status')
FLAGGED=$(echo "$RESP" | jq -r '.data.flagged')
assert_eq "MICR failure reaches analyzing" "$STATUS" "analyzing"
assert_eq "MICR failure is flagged" "$FLAGGED" "true"
echo ""

# ── Test f: Duplicate detection ───────────────────────────────────────────────
# ACC-SOFI-1004 always returns "duplicate_found" from vendor stub — idempotent.
echo "f. Duplicate detection (ACC-SOFI-1004)"
RESP=$(submit_deposit "ACC-SOFI-1004" "60000")
STATUS=$(echo "$RESP" | jq -r '.data.status')
assert_eq "duplicate account is rejected" "$STATUS" "rejected"
echo ""

# ── Test g: Amount mismatch — flagged ─────────────────────────────────────────
echo "g. Amount mismatch flagged (ACC-SOFI-1005)"
RESP=$(submit_deposit "ACC-SOFI-1005" "80000")
STATUS=$(echo "$RESP" | jq -r '.data.status')
FLAGGED=$(echo "$RESP" | jq -r '.data.flagged')
FLAG_REASON=$(echo "$RESP" | jq -r '.data.flag_reason')
assert_eq "amount mismatch reaches analyzing" "$STATUS" "analyzing"
assert_eq "amount mismatch is flagged" "$FLAGGED" "true"
assert_eq "flag reason is amount_mismatch" "$FLAG_REASON" "amount_mismatch"
echo ""

# ── Test h: Auth required — no token → 401 ───────────────────────────────────
echo "h. Auth required — no token returns 401"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/api/v1/deposits" \
    -F "account_id=ACC-SOFI-1006" \
    -F "amount_cents=100000")
assert_eq "missing token returns 401" "$HTTP_CODE" "401"
echo ""

# ── Test i: GET /deposits/:id — transfer with state_history ──────────────────
echo "i. GET /deposits/:id — state_history present"
DETAIL=$(curl -s "$BASE/api/v1/deposits/$HAPPY_ID" \
    -H "Authorization: Bearer $TOKEN")
DETAIL_STATUS=$(echo "$DETAIL" | jq -r '.data.status')
HISTORY_LEN=$(echo "$DETAIL" | jq '.data.state_history | length')
assert_eq "GET by ID returns correct status" "$DETAIL_STATUS" "funds_posted"
if [ "$HISTORY_LEN" -gt "0" ]; then
    pass "state_history is non-empty (length: $HISTORY_LEN)"
else
    fail "state_history is empty (want > 0, got $HISTORY_LEN)"
fi
echo ""

# ── Test j: GET /deposits/:id/images/front — serves image ────────────────────
echo "j. GET /deposits/:id/images/front — returns HTTP 200"
IMG_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE/api/v1/deposits/$HAPPY_ID/images/front" \
    -H "Authorization: Bearer $TOKEN")
assert_eq "front image endpoint returns 200" "$IMG_CODE" "200"
echo ""

# ── Test k: Over-limit rejection (>$5,000) ────────────────────────────────────
# ACC-SOFI-0000 is a basic-pass account; over-limit is enforced at handler level.
# The handler returns 422 before any transfer is created, so we check HTTP code.
echo "k. Over-limit rejection (600000 cents — handler validates before service)"
RESP=$(curl -s -w '\n%{http_code}' -X POST "$BASE/api/v1/deposits" \
    -H "Authorization: Bearer $TOKEN" \
    -F "account_id=ACC-SOFI-0000" \
    -F "amount_cents=600000" \
    -F "front_image=@$FRONT" \
    -F "back_image=@$BACK")
BODY=$(echo "$RESP" | head -n1)
HTTP_CODE=$(echo "$RESP" | tail -n1)
CODE_FIELD=$(echo "$BODY" | jq -r '.code')
assert_eq "over-limit returns 422" "$HTTP_CODE" "422"
assert_eq "over-limit code is DEPOSIT_OVER_LIMIT" "$CODE_FIELD" "DEPOSIT_OVER_LIMIT"
echo ""

# ── Summary ───────────────────────────────────────────────────────────────────
TOTAL=$((PASS+FAIL))
echo "=============================="
echo " Results: $PASS/$TOTAL tests passed"
echo "=============================="

if [ "$FAIL" -gt "0" ]; then
    exit 1
fi
