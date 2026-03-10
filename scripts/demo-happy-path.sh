#!/bin/bash
# demo-happy-path.sh — Full happy-path lifecycle demo:
# Submit clean-pass deposit → settlement → completed → ledger view
# Runs against a running docker compose stack.
# Usage: ./scripts/demo-happy-path.sh

set -euo pipefail

BASE="http://localhost:8080"
TOKEN="tok_investor_test"
OP="OP-001"
FRONT="scripts/fixtures/check-front.png"
BACK="scripts/fixtures/check-back.png"
TODAY=$(date -u +%Y-%m-%d)

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

echo "=============================="
echo " DEMO: Happy Path"
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

# Use a random amount to avoid duplicate hash collisions on re-run.
AMOUNT=$((10000 + RANDOM % 90000))
echo "Using deposit amount: ${AMOUNT} cents"
echo ""

# ── Step 1: Submit clean-pass deposit ────────────────────────────────────────
echo "Step 1: Submit clean-pass deposit (ACC-SOFI-1006, ${AMOUNT} cents)"
RESP=$(curl -s -X POST "$BASE/api/v1/deposits" \
    -H "Authorization: Bearer $TOKEN" \
    -F "account_id=ACC-SOFI-1006" \
    -F "amount_cents=$AMOUNT" \
    -F "front_image=@$FRONT" \
    -F "back_image=@$BACK")

TRANSFER_ID=$(echo "$RESP" | jq -r '.data.transfer_id')
STATUS=$(echo "$RESP" | jq -r '.data.status')
echo "  Transfer ID : $TRANSFER_ID"
echo "  Status      : $STATUS"
assert_eq "deposit reaches funds_posted" "$STATUS" "funds_posted"
echo ""

# ── Step 2: Verify state history ─────────────────────────────────────────────
echo "Step 2: GET /deposits/$TRANSFER_ID — verify state history"
DETAIL=$(curl -s "$BASE/api/v1/deposits/$TRANSFER_ID" \
    -H "Authorization: Bearer $TOKEN")
DETAIL_STATUS=$(echo "$DETAIL" | jq -r '.data.status')
HISTORY_LEN=$(echo "$DETAIL" | jq '.data.state_history | length')
assert_eq "GET by ID returns funds_posted" "$DETAIL_STATUS" "funds_posted"
if [ "$HISTORY_LEN" -ge 1 ]; then
    pass "state_history is non-empty (length: $HISTORY_LEN)"
else
    fail "state_history is empty (want >= 1, got $HISTORY_LEN)"
fi
echo ""

# ── Step 3: Trigger settlement ────────────────────────────────────────────────
echo "Step 3: Trigger EOD settlement for $TODAY"
SETTLE=$(curl -s -X POST "$BASE/api/v1/operator/settlement/trigger" \
    -H "X-Operator-ID: $OP" \
    -H "Content-Type: application/json" \
    -d "{\"batch_date\": \"$TODAY\"}")

SETTLE_COUNT=$(echo "$SETTLE" | jq -r '.data.deposit_count')
SETTLE_TOTAL=$(echo "$SETTLE" | jq -r '.data.total_amount_cents')
SETTLE_STATUS=$(echo "$SETTLE" | jq -r '.data.status')
SETTLE_BATCH_ID=$(echo "$SETTLE" | jq -r '.data.batch_id')
echo "  Batch ID    : $SETTLE_BATCH_ID"
echo "  Deposits    : $SETTLE_COUNT"
echo "  Total cents : $SETTLE_TOTAL"
echo "  Status      : $SETTLE_STATUS"
assert_gte "settlement batch includes at least 1 deposit" "$SETTLE_COUNT" "1"
assert_eq "batch status is submitted" "$SETTLE_STATUS" "submitted"
echo ""

# ── Step 4: Verify deposit is now completed ───────────────────────────────────
echo "Step 4: Verify deposit $TRANSFER_ID is now completed"
FINAL=$(curl -s "$BASE/api/v1/deposits/$TRANSFER_ID" \
    -H "Authorization: Bearer $TOKEN")
FINAL_STATUS=$(echo "$FINAL" | jq -r '.data.status')
FINAL_BATCH=$(echo "$FINAL" | jq -r '.data.settlement_batch_id // empty')
echo "  Final status       : $FINAL_STATUS"
echo "  Settlement batch ID: $FINAL_BATCH"
assert_eq "deposit is now completed" "$FINAL_STATUS" "completed"
assert_nonempty "settlement_batch_id is set" "$FINAL_BATCH"
echo ""

# ── Step 5: View ledger entries ────────────────────────────────────────────────
echo "Step 5: GET /ledger/ACC-SOFI-1006 — verify DEPOSIT entry"
LEDGER=$(curl -s "$BASE/api/v1/ledger/ACC-SOFI-1006" \
    -H "Authorization: Bearer $TOKEN")

ENTRY_COUNT=$(echo "$LEDGER" | jq '[.data.entries[] | select(.transfer_id == "'"$TRANSFER_ID"'")] | length')
if [ "$ENTRY_COUNT" -ge 1 ]; then
    pass "ledger has at least 1 entry for transfer $TRANSFER_ID (count: $ENTRY_COUNT)"
else
    fail "ledger has no entries for transfer $TRANSFER_ID"
fi

DEPOSIT_AMOUNT=$(echo "$LEDGER" | jq -r '[.data.entries[] | select(.transfer_id == "'"$TRANSFER_ID"'" and .sub_type == "DEPOSIT")] | .[0].amount_cents // "0"')
if [ "$DEPOSIT_AMOUNT" != "0" ] && [ "$DEPOSIT_AMOUNT" != "null" ]; then
    assert_eq "DEPOSIT entry amount matches submitted amount" "$DEPOSIT_AMOUNT" "$AMOUNT"
else
    fail "no DEPOSIT entry found for transfer $TRANSFER_ID"
fi
echo ""

# ── Summary ───────────────────────────────────────────────────────────────────
TOTAL=$((PASS+FAIL))
echo "=============================="
echo " Results: $PASS/$TOTAL tests passed"
echo "=============================="

if [ "$FAIL" -gt "0" ]; then
    exit 1
fi
