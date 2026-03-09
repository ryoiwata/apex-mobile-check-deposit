#!/bin/bash
# Phase 11 acceptance tests — settlement engine: X9 file generation, EOD cutoff, batch tracking.
# Runs against an already-running Docker Compose stack.
# Usage: ./scripts/tests/test-phase-11-settlement-engine.sh

set -euo pipefail

BASE="http://localhost:8080"
INVESTOR_TOKEN="tok_investor_test"
OPERATOR_ID="OP-001"
FRONT="scripts/fixtures/check-front.png"
BACK="scripts/fixtures/check-back.png"
TODAY=$(date +%Y-%m-%d)

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

assert_nonempty() {
    local label="$1" got="$2"
    if [ -n "$got" ] && [ "$got" != "null" ] && [ "$got" != "" ]; then
        pass "$label (got: $got)"
    else
        fail "$label (want: non-empty, got: '$got')"
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

operator_post() {
    local path="$1" body="$2"
    curl -s -X POST "$BASE/api/v1/operator/$path" \
        -H "X-Operator-ID: $OPERATOR_ID" \
        -H "Content-Type: application/json" \
        -d "$body"
}

trigger_settlement() {
    curl -s -X POST "$BASE/api/v1/operator/settlement/trigger" \
        -H "X-Operator-ID: $OPERATOR_ID" \
        -H "Content-Type: application/json" \
        -d "{\"batch_date\": \"$TODAY\"}"
}

get_deposit() {
    local id="$1"
    curl -s "$BASE/api/v1/deposits/$id" \
        -H "Authorization: Bearer $INVESTOR_TOKEN"
}

echo "=============================="
echo " Phase 11 Acceptance Tests"
echo " $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
echo " Batch date: $TODAY"
echo "=============================="
echo ""

# ── Setup: Flush Redis to clear stale duplicate hashes ───────────────────────
echo "Setup: Flushing Redis to clear stale duplicate-check hashes"
FLUSH_OK=0
# Try the Docker socket path that works in this environment
if DOCKER_HOST=unix:///var/run/docker.sock docker exec apex-mobile-check-deposit-redis-1 redis-cli FLUSHALL >/dev/null 2>&1; then
    echo "  Redis flushed (DOCKER_HOST=unix:///var/run/docker.sock)"
    FLUSH_OK=1
elif docker exec apex-mobile-check-deposit-redis-1 redis-cli FLUSHALL >/dev/null 2>&1; then
    echo "  Redis flushed (default docker)"
    FLUSH_OK=1
fi
if [ "$FLUSH_OK" -eq 0 ]; then
    echo "  WARNING: Redis flush failed — duplicate hashes from prior runs may cause false rejections"
fi
echo ""

# Use non-overlapping random amount ranges so clean-pass deposits never share
# the same (routing, account, amount, serial) hash even across retries.
# Vendor stub always returns routing=021000021, account=123456789, serial=0001 for clean pass.
AMOUNT1=$((10000 + RANDOM % 9000))   # 10000–18999
AMOUNT2=$((20000 + RANDOM % 9000))   # 20000–28999
AMOUNT3=$((40000 + RANDOM % 9000))   # 40000–48999
AMOUNT_FLAGGED=$((50000 + RANDOM % 9000))  # 50000–58999

# ── Test a: Clean-pass deposit 1 → funds_posted ───────────────────────────────
echo "a. Clean-pass deposit 1 (ACC-SOFI-1006, ${AMOUNT1} cents) — assert funds_posted"
RESP1=$(submit_deposit "ACC-SOFI-1006" "$AMOUNT1")
STATUS1=$(echo "$RESP1" | jq -r '.data.status')
ID1=$(echo "$RESP1" | jq -r '.data.transfer_id')
assert_eq "deposit 1 reaches funds_posted" "$STATUS1" "funds_posted"
echo ""

# ── Test b: Clean-pass deposit 2 → funds_posted ───────────────────────────────
echo "b. Clean-pass deposit 2 (ACC-SOFI-0000, ${AMOUNT2} cents) — assert funds_posted"
RESP2=$(submit_deposit "ACC-SOFI-0000" "$AMOUNT2")
STATUS2=$(echo "$RESP2" | jq -r '.data.status')
ID2=$(echo "$RESP2" | jq -r '.data.transfer_id')
assert_eq "deposit 2 reaches funds_posted" "$STATUS2" "funds_posted"
echo ""

# ── Test c: Rejected deposit (IQA blur) — should NOT appear in settlement ─────
echo "c. Rejected deposit (ACC-SOFI-1001) — assert rejected, excluded from settlement"
RESP_REJ=$(submit_deposit "ACC-SOFI-1001" "35000")
STATUS_REJ=$(echo "$RESP_REJ" | jq -r '.data.status')
ID_REJ=$(echo "$RESP_REJ" | jq -r '.data.transfer_id')
assert_eq "blur deposit is rejected" "$STATUS_REJ" "rejected"
echo ""

# ── Test d: Trigger settlement — deposit_count >= 2 ──────────────────────────
echo "d. Trigger settlement for $TODAY — assert deposit_count >= 2"
SETTLE1=$(trigger_settlement)
SETTLE1_COUNT=$(echo "$SETTLE1" | jq -r '.data.deposit_count')
SETTLE1_TOTAL=$(echo "$SETTLE1" | jq -r '.data.total_amount_cents')
SETTLE1_PATH=$(echo "$SETTLE1" | jq -r '.data.file_path // empty')
SETTLE1_STATUS=$(echo "$SETTLE1" | jq -r '.data.status')
SETTLE1_BATCH_ID=$(echo "$SETTLE1" | jq -r '.data.batch_id')
assert_gte "settlement includes at least 2 deposits" "$SETTLE1_COUNT" "2"
echo ""

# ── Test e: First deposit is now completed ────────────────────────────────────
echo "e. Deposit 1 ($ID1) — assert status=completed after settlement"
DETAIL1=$(get_deposit "$ID1")
POST_STATUS1=$(echo "$DETAIL1" | jq -r '.data.status')
POST_BATCH1=$(echo "$DETAIL1" | jq -r '.data.settlement_batch_id // empty')
assert_eq "deposit 1 is completed" "$POST_STATUS1" "completed"
assert_nonempty "deposit 1 has settlement_batch_id" "$POST_BATCH1"
echo ""

# ── Test f: Second deposit is now completed ───────────────────────────────────
echo "f. Deposit 2 ($ID2) — assert status=completed after settlement"
DETAIL2=$(get_deposit "$ID2")
POST_STATUS2=$(echo "$DETAIL2" | jq -r '.data.status')
POST_BATCH2=$(echo "$DETAIL2" | jq -r '.data.settlement_batch_id // empty')
assert_eq "deposit 2 is completed" "$POST_STATUS2" "completed"
assert_nonempty "deposit 2 has settlement_batch_id" "$POST_BATCH2"
echo ""

# ── Test g: Rejected deposit remains rejected, not completed ──────────────────
echo "g. Rejected deposit ($ID_REJ) — assert still rejected (not moved to completed)"
DETAIL_REJ=$(get_deposit "$ID_REJ")
POST_STATUS_REJ=$(echo "$DETAIL_REJ" | jq -r '.data.status')
assert_eq "rejected deposit stays rejected" "$POST_STATUS_REJ" "rejected"
echo ""

# ── Test h: Settlement total >= sum of our two deposits ───────────────────────
echo "h. Settlement total_amount_cents ($SETTLE1_TOTAL) >= AMOUNT1+AMOUNT2 ($((AMOUNT1+AMOUNT2)))"
EXPECTED_MIN=$((AMOUNT1 + AMOUNT2))
assert_gte "total_amount_cents covers both clean deposits" "$SETTLE1_TOTAL" "$EXPECTED_MIN"
assert_eq "batch status is submitted" "$SETTLE1_STATUS" "submitted"
echo ""

# ── Test i: Settlement file path returned and non-empty ───────────────────────
echo "i. Settlement file path — assert non-empty in batch response"
assert_nonempty "settlement file_path is present" "$SETTLE1_PATH"
echo ""

# ── Test j: Second settlement trigger — deposit_count = 0 (all already batched) ─
echo "j. Second settlement trigger — assert deposit_count=0 (idempotent)"
SETTLE2=$(trigger_settlement)
SETTLE2_COUNT=$(echo "$SETTLE2" | jq -r '.data.deposit_count')
assert_eq "second trigger finds 0 eligible deposits" "$SETTLE2_COUNT" "0"
echo ""

# ── Test k: Third deposit submitted after first batch — deposit_count = 1 ─────
echo "k. Submit deposit 3 after settlement (ACC-SOFI-0000, ${AMOUNT3} cents), then trigger again"
RESP3=$(submit_deposit "ACC-SOFI-0000" "$AMOUNT3")
STATUS3=$(echo "$RESP3" | jq -r '.data.status')
ID3=$(echo "$RESP3" | jq -r '.data.transfer_id')
assert_eq "deposit 3 reaches funds_posted" "$STATUS3" "funds_posted"

SETTLE3=$(trigger_settlement)
SETTLE3_COUNT=$(echo "$SETTLE3" | jq -r '.data.deposit_count')
assert_eq "third settlement picks up exactly 1 new deposit" "$SETTLE3_COUNT" "1"
echo ""

# ── Test l: Third deposit is now completed ────────────────────────────────────
echo "l. Deposit 3 ($ID3) — assert status=completed after third settlement"
DETAIL3=$(get_deposit "$ID3")
POST_STATUS3=$(echo "$DETAIL3" | jq -r '.data.status')
POST_BATCH3=$(echo "$DETAIL3" | jq -r '.data.settlement_batch_id // empty')
assert_eq "deposit 3 is completed" "$POST_STATUS3" "completed"
assert_nonempty "deposit 3 has settlement_batch_id" "$POST_BATCH3"
echo ""

# ── Test m: Operator-approved flagged deposit is included in settlement ────────
echo "m. Flagged deposit (ACC-SOFI-1003) operator-approved → included in settlement"

# Submit MICR-failure deposit → analyzing (flagged)
RESP_FLAG=$(submit_deposit "ACC-SOFI-1003" "$AMOUNT_FLAGGED")
STATUS_FLAG=$(echo "$RESP_FLAG" | jq -r '.data.status')
FLAGGED_VAL=$(echo "$RESP_FLAG" | jq -r '.data.flagged')
ID_FLAG=$(echo "$RESP_FLAG" | jq -r '.data.transfer_id')
assert_eq "flagged deposit reaches analyzing" "$STATUS_FLAG" "analyzing"
assert_eq "flagged deposit is flagged=true" "$FLAGGED_VAL" "true"

# Operator approves it → funds_posted
APPROVE_RESP=$(operator_post "deposits/$ID_FLAG/approve" \
    '{"notes":"MICR verified manually by operator"}')
APPROVE_STATUS=$(echo "$APPROVE_RESP" | jq -r '.data.status')
assert_eq "operator-approved deposit reaches funds_posted" "$APPROVE_STATUS" "funds_posted"

# Fourth settlement trigger — must include the newly approved deposit
SETTLE4=$(trigger_settlement)
SETTLE4_COUNT=$(echo "$SETTLE4" | jq -r '.data.deposit_count')
assert_gte "fourth settlement includes operator-approved deposit" "$SETTLE4_COUNT" "1"

# Verify the approved deposit is now completed
DETAIL_FLAG=$(get_deposit "$ID_FLAG")
POST_STATUS_FLAG=$(echo "$DETAIL_FLAG" | jq -r '.data.status')
assert_eq "operator-approved deposit is completed after settlement" "$POST_STATUS_FLAG" "completed"
echo ""

# ── Summary ───────────────────────────────────────────────────────────────────
TOTAL=$((PASS+FAIL))
echo "=============================="
echo " Results: $PASS/$TOTAL tests passed"
echo "=============================="

if [ "$FAIL" -gt "0" ]; then
    exit 1
fi
